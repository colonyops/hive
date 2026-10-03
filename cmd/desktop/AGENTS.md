# Agent Instructions — Hive Desktop

Scope: `cmd/desktop/` and everything below it. The repository-root `AGENTS.md`
also applies.

Two references, neither duplicated here — read them instead:

- **`docs/architecture.md`** — read before adding a subsystem, an entrypoint,
  or an extension point. It names the pattern each part of the app follows.
  Where it and the code disagree, it wins for new work.
- **`cmd/desktop/README.md`** — the long-form reference: native shell, pinned
  versions, settings, icons, the actions catalog, the e2e harness.

## What this app is

A Wails v3 shell (Vue 3 + TypeScript frontend, Go backend) rendering a
GitHub-backed feed. A **flow** (`flows/*.yaml`) wires `sources.*` nodes through
filters into `feed`, `action`, and `notify` terminals. A background producer
polls sources and appends to an event log; the engine commits `feed_item` rows
and durable `output_command`s that an output worker dispatches.

GitHub is a **connector, not a login** — nothing in the app is gated on holding
a credential.

## Commands

```bash
mise run desktop:dev           # Wails with this worktree's launch.env
mise run desktop:serve         # headless HTTP build on :8080 — the agent UI loop
mise run desktop:test          # frontend vitest + the desktop's Go packages
mise run desktop:frontend:lint # ESLint (type-aware); see "Frontend lint and format"
mise run desktop:bindings      # regenerate TS bindings after a Wails service change
mise run desktop:e2e           # Docker-only Playwright gate
mise run desktop:devserver     # the shared GitHub proxy `dev` routes through
```

`desktop:dev:prepare` / `desktop:dev:fresh` / `desktop:dev:reset` manage this worktree's isolated
instance. `solo up` brings up devserver + app together from `.solo.yml`.

## Never

- **Never run Playwright or the e2e harness on the host.** `mise run desktop:e2e` is
  Docker-only and there is no host fallback.
- **Never verify UI by hand in a GUI build.** The headless `mise run desktop:serve`
  build driven with browser tooling is the default loop. When the question is
  about the real app (the native window, the WKWebView, shell integration),
  drive `mise run desktop:dev` through its Wails MCP server with the
  `desktop-ui-audit` skill, never by clicking or by swapping a hand-built
  binary. Assets are `//go:embed`ded, so a frontend edit needs a re-run of
  `serve`; `desktop:dev` is the Vite HMR loop.
- **Never edit generated files** — `frontend/bindings/`, `data/queries/models.go`,
  `data/queries/*.sql.go`, `*_enum.go`.
- **Never add `init()`.** `gochecknoinits` is on; use package-variable
  initialization (`var _ = registerEvents()`).
- **Never put flow-node execution in the frontend.** Execution is Go's
  (`cmd/desktop/internal/app/runtime`); the frontend lint fails if a
  `nodes/*/runtime.ts` reappears or a node module imports one.
- **Never call an `emit*` helper from the core.** They are unexported and
  `forbidigo` fails the build.
- **Never import Wails or `cmd/desktop/internal/adapter` from `cmd/desktop/internal/app`.** `depguard`
  enforces it.
- **Never put a token in config.** `flows/` is dotfiles-managed; config holds
  credential refs only.
- **Never gate the app on being connected to GitHub.**
- **Never `go build ./cmd/desktop`** without `-o` — the package is `main` and named `desktop`, so
  it drops a stray `desktop` binary in the current directory. Use
  `go build -o ./cmd/desktop/bin/hive-desktop ./cmd/desktop` (`-tags server` for
  headless). The mise tasks already do this.

## Architecture rules

- **The core publishes typed payloads; the Wails boundary degrades them to
  wake-up signals.** On receipt the frontend re-reads the service. Adding an
  event is three things: a payload type in `app/events/events.go`, a publish
  from the core, and a subscriber in `wailsui/events.go`.
- **`inbox:updated` is the feed's signal, not `log:appended`.** A log row may
  route nowhere; `inbox:updated` fires after the engine commits, which is when
  items are readable.
- Subscriptions use `events.Coalesce()`. A consumer needing every event in order
  uses `events.Buffer(n)` — the delta is in the payload.
- **A source connector is declared in Go.** `cmd/desktop/internal/app/sources/registry.go`
  is the whole map; the flow node registry, the runtime behaviour registry, and
  Settings ▸ Integrations all derive from a `connector.Descriptor`. A new
  connector needs a `nodes/<type>/` editor entry here and nothing else.
- **Flows and actions hot-reload, last-good.** A broken file keeps the previous
  set rather than blanking the running app.
- **LLM prompt text is Go-owned** — `cmd/desktop/internal/app/prompts/templates/`. Nothing
  in the frontend builds a prompt string. Per-type prose belongs in
  `flow/docs/<type>.md`, and a bijection test enforces that a new type
  documents itself.
- **Node docs cross the language boundary.** The frontend imports
  `cmd/desktop/internal/app/flow/docs/*.md` via the `@nodedocs` alias, declared in **both**
  `vite.config.ts` and `vitest.config.ts`. An LLM reads them too, so keep them
  free of UI-only references like "the row below".

## Code generation

Run `mise run generate` (sqlc, enums) and commit the output alongside its input.

`mise run desktop:bindings` runs the Wails CLI from `cmd/desktop/`, so the CLI
treats that directory as the app package. Binding method ids hash the Go package
path, so _moving_ a service invalidates them; `mise run desktop:check:bindings` catches
it.

## Testing

`mise run desktop:test` is the default gate. `data` and `runtime` tests
use real SQLite.

Engine behaviour changes — routing, sink tagging, node-run accounting — belong
in a fixture under `cmd/desktop/internal/app/runtime/testdata/parity/*.json`: a flow, a
batch of messages, and the exact `CommitBatch` they are worth.

## Frontend lint and format

`mise run desktop:frontend:lint` and `mise run desktop:frontend:format:check`
gate the frontend; `npm run format` fixes formatting. Every lint rule is an
error. `frontend/eslint-suppressions.json` holds the violations that existed
when the lint landed: fix one, then run `npx eslint --prune-suppressions .`
in `frontend/`, because a stale entry fails the lint. Never add entries to it
or `--suppress-all` new code; fix the code or disable the rule on that line
with a reason.

## Frontend shared state

Shared state is a store under `frontend/src/stores/`, one `defineStore` per
file. Composables under `frontend/src/composables/` hold no shared state: they
are per-instance or stateless. `frontend/src/stores/README.md` is the
standard: when something is a store, how to write one, the building blocks
(`useResource` for anything fetched), the conventions (`reload`, errors as
`string | null`, readonly state), and how to test one with `resetStores()`.
The lint enforces the split; the composables that predate it sit in
`eslint-suppressions.json` until each one migrates (#536).

## Frontend components

Reusable building blocks (buttons, overlays, form controls, banners) live in
`frontend/src/components/ui/`; feature components stay in `components/` or a
feature folder. `frontend/src/components/ui/README.md` is the index and the
rules: which block to use, the overlay and test-id conventions, and how to add
a block. Read it before writing a button, a dialog, or an error line.

## Frontend DOM, timers, and storage

Reach for `@vueuse/core` first: `useEventListener`, `onKeyStroke`,
`onClickOutside`, `useResizeObserver`, `useWindowSize`, `useIntervalFn`,
`useStorage`. Each one ends with the component's scope, so there is no
`onUnmounted` to forget. To attach a listener or observer only while
something holds, pass a getter target (`() => (open.value ? window : null)`)
rather than adding and removing it by hand. The lint rejects raw
`addEventListener`, `new ResizeObserver`, `setInterval`, and `localStorage`
in `.vue` files.

Raw browser APIs stay where VueUse has no scope to bind to or does not fit:
module singletons (`useFrameStats`, `useSessionStatuses`, `usePerf`), helpers
that hand back a disposer for an xterm host (`lib/terminal*.ts`,
`useTerminalWindows`), a drag started from a pointer handler (`startDrag`),
and storage read once with validation (`useTheme`, `useFeedState`).

The app's own focus and overlay composables have no VueUse equivalent and
stay: `useFocusTrap` (VueUse's needs `focus-trap`, and ours deliberately
leaves teleported popovers out), `useReturnFocus`, `useAutofocus` (it also
focuses components that expose `focus()`), `useAnchoredPopover`,
`useResizablePanel`, and `useEscapeToClose` (one Escape stack: only the
topmost enabled caller fires, so a stacked overlay never closes the one under
it). Every modal surface is a `BaseModal` or a `DrawerSheet`; both register in
`useOpenModalCount`, which gates the global keybindings. `useClipboard` goes
through the Wails clipboard because `navigator.clipboard` no-ops in WKWebView
when the document is not focused.

## Mock modes

`HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE`: `feed` / `pipeline` / `action-smoke`
start with `github/octocat` connected and seed fixed rows; `onboarding` starts
with nothing connected and grants a fake device flow after ~1.5s. Unset means
live backends. The live producer and output worker are skipped in all of them.

**A mock connection must write the credential it pretends to hold**, not just
flip a status flag — everything that resolves an account off the credential
store works live and silently fails otherwise.

## Release notes

A user-visible change adds a fragment to
`cmd/desktop/releasenotes/changelog/unreleased/` **in the PR that earns it**,
with `mise run changelog:new -- --product desktop --kind <added|changed|fixed> "..."`
(the `release-notes` skill). One file
per change is what keeps concurrent branches from conflicting over the
changelog (ADR release-notes-accumulate-as-fragments). The fragments render as
the draft, and a release promotes every program's draft to its
`changelog/<version>.md` -- which is also where it is consolidated and
given its summary. The release gate refuses a version with no entry
(ADR release-notes-ship-inside-the-binary).

## Settings and environment

The canonical shape is the `settings.Settings` struct
(`cmd/desktop/internal/app/settings/settings.go`): `yaml:` tags for keys, doc comments for
meaning, `DefaultSettings()` for what ships, `env:` tags for the
`HIVE_DESKTOP_*` overrides. It is the only complete list — do not copy it.

Adding a setting also means updating `cmd/desktop/README.md` and
`cmd/desktop/internal/app/prompts/templates/settings.tmpl`, which an LLM reads and cannot
resolve from the struct.

Variables read outside that struct — the data/config roots, `HIVE_GITHUB_TOKEN`,
the devtools and e2e markers — are documented in `cmd/desktop/README.md`.
