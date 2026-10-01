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
mise run desktop:bindings      # regenerate TS bindings after a Wails service change
mise run desktop:e2e           # Docker-only Playwright gate
mise run desktop:devserver     # the shared GitHub proxy `dev` routes through
```

`desktop:dev:prepare` / `desktop:dev:fresh` / `desktop:dev:reset` manage this worktree's isolated
instance. `solo up` brings up devserver + app together from `.solo.yml`.

## Never

- **Never run Playwright or the e2e harness on the host.** `mise run desktop:e2e` is
  Docker-only and there is no host fallback.
- **Never verify UI with a local GUI build.** Use `mise run desktop:serve` and drive it
  with browser tooling. Assets are `//go:embed`ded, so a frontend edit needs a
  re-run; use `desktop:dev` for a Vite HMR loop instead.
- **Never edit generated files** — `frontend/bindings/`, `data/queries/models.go`,
  `data/queries/*.sql.go`, `*_enum.go`.
- **Never add `init()`.** `gochecknoinits` is on; use package-variable
  initialization (`var _ = registerEvents()`).
- **Never put flow-node execution in the frontend.** Execution is Go's
  (`cmd/desktop/internal/app/runtime`); `pipeline/__tests__/import-hygiene.spec.ts` fails if
  a `nodes/*/runtime.ts` reappears.
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
with `mise run desktop:changelog:new -- --kind <added|changed|fixed> "..."`. One file
per change is what keeps concurrent branches from conflicting over the
changelog (ADR release-notes-accumulate-as-fragments). The fragments render as
the draft; prereleases publish it as it stands, and a stable release promotes
it to `changelog/<version>.md` -- which is also where it is consolidated and
given its summary. The release gate refuses a stable version with no entry
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
