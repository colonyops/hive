# goja for function nodes, behind a ScriptRuntime port

- **Status:** accepted
- **Date:** 2026-07-25

## Context

A `function` node runs user-authored code against every message that reaches it. Until now that code was JavaScript evaluated by `new Function(...)` inside a Web Worker, which meant flow execution needed a browser. Moving the engine into Go (ADR flow-engine-in-go) needs a script engine that runs in the same process.

Two constraints narrowed the field before preference did. The headless `-tags server` build is CGO-free — the same reason the repo is on `modernc.org/sqlite` — so a cgo engine such as QuickJS would take that build away. And the stated requirement for the node is that a user can do *anything* with flexible JSON, which is what a payload is.

Lua was the obvious alternative and looked cheaper, on the assumption that goja is awkward with dynamic JSON. That assumption is inverted: goja's friction is with *struct* interop, not with `map[string]any`. It is Lua that is weak on exactly the stated axis — a table cannot distinguish `[]` from `{}`, a bare `nil` deletes a key rather than storing JSON `null`, and `pairs()` has no defined order, so a payload hash would need sorting to be stable. Choosing Lua would also mean rewriting every existing function node, its documentation, and the webhook transform prompt.

The node's lifecycle was a second question. It had `on_start` and `on_stop` hooks alongside `on_message`, which the browser transport invoked around a worker's life.

## Decision

`function` nodes execute through **goja**, a pure-Go ECMAScript interpreter, behind a `ScriptRuntime` / `ScriptInstance` port declared in `internal/app/runtime` with an explicit registry. goja is the only implementation; the port exists so a second language is an implementation plus one `Register` call rather than an engine change. There is deliberately no `runtime:` field on the node — a config key for a choice that has one option is speculative, and the port is what makes adding the field later cheap.

Four things are fixed by this decision:

**`on_message` is the whole lifecycle.** The node does no I/O, so `on_stop` could only mutate state that is about to be discarded, and `on_start` is better written as lazy initialization inside `on_message` (`state.counts ??= {}`). One entry point also removes the pretence that `msg` may be undefined.

**Outputs are port-indexed.** The port's canonical return is `[][]store.Msg`. JavaScript's return shapes overlap syntactically — `Msg[]` and a port-indexed array are both plain arrays — and the *language adapter* resolves that using the node's declared output count. The engine never sees an ambiguous value, so a second language brings its own ambiguities without teaching the engine about them.

**Values cross the boundary as JSON, through the VM's own `JSON.parse` and `JSON.stringify`.** Not `vm.ToValue`: goja's wrapper for a Go slice answers `false` to `Array.isArray`, and a Go map's key order is a map's. `JSON.parse` produces genuine JavaScript values, and `null`, `[]`, `{}` and key order all survive the round trip. This is the property that ruled Lua out, so it is worth paying a marshal for.

**Timeouts are cooperative, and bounded rather than guaranteed.** `context.AfterFunc` plus `vm.Interrupt` replaces the browser's `worker.terminate()`. Evaluation runs on a goroutine the runtime is willing to abandon, so `OnMessage` returns even when an interrupt does not land; the abandoned goroutine holds its slot in a process-wide `ScriptPool` until it actually returns. A node that times out is dropped and respawned with fresh state, which is what the browser's "terminate, respawn" did.

## Consequences

- `worker.terminate()` was a hard kill and this is not. A script that neither allocates nor returns to the interpreter can outlive its interrupt. Its message is still discarded as an error and its flow keeps running; what it consumes is part of a fixed budget, and once that budget is spent further evaluations fail fast with a visible `unavailable` error rather than hanging. This is a known, accepted limitation.
- **`on_start` and `on_stop` are gone from `FunctionConfig`.** A flow file that still carries either key now fails to load — node decoding is strict about unknown fields. Setup moves into `on_message`.
- That deletion also removed `fields/TabStrip.vue`, whose only consumer was the function editor's three-tab layout.
- The message envelope is a fixed set of fields (`ID`, `Key`, `Topic`, `Ts`, `Payload`, `Snapshot`, `SourceKind`, `SourceScope`, `OccurrenceKey`). A property attached to `msg` itself is not carried downstream, because that envelope is also what an HTTP or MCP surface will serialise. `msg.Payload` is opaque and passes through whole, which is where per-message data belongs.
- Compile and runtime failures normalise to one `ScriptError` with a `Kind` and the author's own line and column — the wrapper's offset is subtracted, and an "unexpected end of input" is clamped to the last line the author wrote rather than reported against a closing brace they cannot see. Syntax checking is a core concern exposed to the editor, so the drawer's live check and the engine share one compiler.
- goja is a new dependency with its own ECMAScript version coverage. A script using a feature goja has not implemented fails at deploy rather than silently, because `NewRunner` compiles every function node up front.
