# goja for function nodes, behind a ScriptRuntime port

- **Status:** accepted
- **Date:** 2026-07-25

## Context

A `function` node runs user-authored code against every message that reaches it, and the flow engine runs
in Go (ADR flow-engine-in-go), so the script engine must run in-process. The headless `-tags server` build
is CGO-free (the reason the repo uses `modernc.org/sqlite`), which rules out cgo engines such as QuickJS.
The node's job is arbitrary work on flexible JSON payloads.

Lua is the obvious alternative and is weak on exactly that axis: a table cannot distinguish `[]` from
`{}`, `nil` deletes a key instead of storing JSON `null`, and `pairs()` has no defined order.

## Decision

`function` nodes execute through **goja**, a pure-Go ECMAScript interpreter, behind the `ScriptRuntime` /
`ScriptInstance` port in `cmd/desktop/internal/app/runtime` with a `ScriptRegistry`. `runtime/js` is the
only implementation; a second language is an implementation plus one `Register` call. There is no
`runtime:` field on the node until a second language exists.

**`on_message` is the whole lifecycle.** The node does no I/O, so there are no start or stop hooks;
initialization is lazy inside `on_message` (`state.counts ??= {}`).

**Outputs are port-indexed.** The engine receives `[][]Msg`. JavaScript's return shapes overlap (`Msg[]`
and a port-indexed array are both arrays), and the language adapter resolves that with the node's
declared output count, so the engine never sees an ambiguous value.

**Values cross the boundary as JSON, through the VM's own `JSON.parse` and `JSON.stringify`**, not
`vm.ToValue`: goja's wrapper for a Go slice fails `Array.isArray`, and a Go map has no key order. With
JSON, `null`, `[]`, `{}`, and key order survive the round trip.

**Timeouts are cooperative and bounded, not guaranteed.** `context.AfterFunc` plus `vm.Interrupt` stops a
script. Evaluation runs on a goroutine the runtime can abandon, so `OnMessage` returns even when an
interrupt does not land; the abandoned goroutine holds its slot in the process-wide `ScriptPool` until it
returns. A node that times out is dropped and respawned with fresh state.

## Consequences

- A script that never returns to the interpreter can outlive its interrupt. Its message is discarded as an
  error and the flow keeps running; once the pool is exhausted, further evaluations fail fast with an
  `unavailable` error instead of hanging.
- Node decoding rejects unknown fields, so a flow file with keys the node does not declare fails to load.
- The message envelope is a fixed set of fields (`ID`, `Key`, `Topic`, `Ts`, `Payload`, `Snapshot`,
  `SourceKind`, `SourceScope`, `OccurrenceKey`). A property attached to `msg` itself is not carried
  downstream; per-message data belongs in `msg.Payload`.
- Compile and runtime failures normalize to one `ScriptError` with a `Kind` and the author's own line and
  column. The editor's live syntax check goes through `ScriptRuntime.Check`, the same compiler the engine
  uses.
- A script using an ECMAScript feature goja lacks fails at deploy, because `NewRunner` compiles every
  function node up front.
