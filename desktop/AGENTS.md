# Agent Instructions — Hive Desktop

Scope: the `desktop/` Wails app and its Go backend under `internal/desktop/**`.
The repository-root `AGENTS.md` still applies (git standards, quality gates,
landing-the-plane). `desktop/README.md` is the long-form reference — native
shell, pinned versions, parent-module adaptations, icons, and the flows/actions
data model. **Read `README.md` before changing native-shell, build, or icon
wiring; do not duplicate its detail here.**

## What this app is

A Wails v3 desktop shell (Vue 3 + TypeScript frontend, Go backend) that renders
a GitHub-backed feed. A **flow** (`flows/*.yaml`) wires `github-source` nodes
through filters into `feed` and `action` terminals; a background producer polls
sources, appends to an event log, and commits durable `feed_item` rows the
sidebar reads. `action` nodes emit durable `output_command`s that an output
worker dispatches (`launch-session`, `shell`, `publish-message`). Auth is a
GitHub OAuth device flow with a PAT fallback, tokens in the OS keychain.

## Code layout

Go — the `desktop/` package is **thin Wails wiring only**; real logic lives in
`internal/desktop/**`:

```
desktop/
  main.go                 # Wails app: services, window, tray, event registration
  *service.go             # Wails service structs exposed to the frontend (RPC surface)
  build/                  # platform Taskfiles, config.yml, icon masters, scripts
  e2e/                    # Docker-only Playwright harness (fixtures, scripts, tests)
  frontend/               # Vue 3 + TS + Vite + Tailwind v4
internal/desktop/
  desktop.go              # env-var surface, data/config/flows/actions paths
  auth/                   # device-flow + PAT auth behind the auth service
  feed/                   # GitHub fetch layer (LiveProvider + mock fixtures)
  pipeline/               # producer, output worker, executors, retention
    actions/              # actions.yml store, watcher, seed, editable model
    flow/                 # flow YAML parse/validate/save, FlowsWatcher, sidebar
    pipelinedb/           # sqlc-backed SQLite: event log, feed_item, output_command
```

Frontend (`frontend/src/`): `App.vue` + `components/` (feed UI), `composables/`
(`useFeedState`, `useAuth`, …), `pipeline/` (the flow editor — canvas, node
palette, in-browser graph engine in `pipeline/engine/`), `lib/` (presentation
helpers), `types/`. TS bindings to Go services are **generated** into
`frontend/bindings/` — see Code generation.

## Development

Drive everything through the **root** mise tasks (canonical entry points):

```bash
mise run desktop:dev       # wails3 dev — live frontend (Vite HMR) + Go; picks a free port
mise run desktop:serve     # headless HTTP server build on localhost:8080 (agent UI loop)
mise run desktop:build     # build the app (macOS emits desktop/bin/hive-desktop)
mise run desktop:generate  # regenerate frontend TS bindings after Go service changes
mise run desktop:icons     # regenerate committed icon assets from SVG masters
mise run desktop:test      # frontend vitest + Go tests (unit)
mise run desktop:e2e       # Docker-only Playwright regression gate
```

Go lint/format is the root `mise run lint` (golangci-lint); frontend type
errors surface via `vue-tsc` in the build. Run quality gates after changes.

### Manual / UI verification

Use the **headless server build** for the agent UI loop — never a local GUI
build:

```bash
mise run desktop:serve                       # serves at http://localhost:8080
HIVE_DESKTOP_MOCK=onboarding mise run desktop:serve   # drive the signed-out first-run screen offline
```

Drive it with Playwright/browser tooling, read screenshots under
`desktop/e2e/screenshots`, edit, repeat. Assets are `//go:embed`ded, so
frontend edits require re-running `desktop:serve`; for a fast frontend loop use
`desktop:dev` (Vite HMR). Native-shell behavior (Dock icon, traffic-light
centering, close-hides-window, tray menu, template-icon tinting) is a **manual**
verification concern — it cannot be checked headlessly.

## Testing

- **Unit** (`mise run desktop:test`): Go logic (`go test ./desktop/...
  ./internal/desktop/...`) + frontend `vitest`. `pipelinedb` tests use real
  SQLite. This is the default gate for backend/frontend changes.
- **E2E** (`mise run desktop:e2e`): **Docker-only.** Builds the digest-pinned
  Go/Playwright image in `desktop/e2e/Dockerfile` and runs Playwright inside it
  against private feed / onboarding / pipeline / action-smoke server instances.

**CRITICAL: never run Playwright or the e2e harness on the host.** `run-docker.sh`
mints a fresh 256-bit harness marker that both the image command and the server
launcher require, so host Playwright cannot start the servers. There is no host
fallback — Docker must be available. Each server gets a private data/config root
so parallel projects never mutate checked-in fixtures or share SQLite state.

## Code generation — never edit generated files by hand

- **sqlc** (`internal/desktop/pipeline/pipelinedb/`): queries in `queries/`,
  migrations in `migrations/*.up.sql`. Regenerate with the root `mise run
  generate`; `models.go` and `queries.sql.go` are committed and generated.
  Commit generated output alongside the SQL change.
- **Wails TS bindings** (`frontend/bindings/`): after changing a Wails service
  method or its types, run `mise run desktop:generate`. Bindings **must** be
  generated with the working directory at `desktop/` so the Wails CLI treats it
  as the app package while Go walks up to the parent module. The Vite plugin and
  typed events depend on these — a stale binding is a frontend type error.

## Patterns and gotchas

- **Single Go module.** `desktop/` has no `go.mod`; it is the
  `github.com/colonyops/hive/desktop` package inside the root module. Because
  the package is `main` and named `desktop`, you **must** give the binary an
  explicit output path — `go build -o ./desktop/bin/hive-desktop ./desktop`
  (add `-tags server` for the headless variant). A bare `go build ./desktop`
  collides with this directory. The mise tasks already do this correctly.
- **No `init()`.** This repo enables `gochecknoinits`. Event registration in
  `main.go` uses package-variable initialization (`var _ = registerEvents()`),
  not `init()`. Follow that pattern.
- **Frontend events are wake-up signals, not payloads.** `main.go` registers
  `auth:updated`, `log:appended`, `flows:updated`, `actions:updated`. On
  receipt the frontend re-reads the relevant service; the event just says
  "something changed" (only `log:appended` carries meaningful data — the new
  tail offset). Adding a new signal means registering it in `registerEvents()`
  and adding an `emit*` helper.
- **Mock modes** (`HIVE_DESKTOP_MOCK`): `feed`/`pipeline`/`action-smoke` start
  authenticated; `onboarding` starts signed out with a fake device flow that
  grants after ~1.5s. Unset → live backends. In mock modes the live producer
  and output-worker background loop are skipped; `feed`/`action-smoke` seed
  fixed `feed_item` rows (see `mockseed.go`). Use these for deterministic
  offline/e2e runs — do not hit real GitHub in tests.
- **Config vs data split.** User-editable config (flows, `actions.yml`) lives in
  `$XDG_CONFIG_HOME/hive/desktop/` (so it can live in a dotfiles repo);
  app-local state (`feed_item`, read state, event-log offsets, queued output
  commands) lives in the data dir's `desktop/` subdirectory. Respect that
  boundary when adding persistence.
- **Flows/actions are code, hot-reloaded and last-good.** Flow parsing is strict
  and validated by Go on save/deploy (unique node ids, known types, source
  limits within GitHub caps, action refs that exist, valid wires). `FlowsWatcher`
  and `ActionsWatcher` watch the *directory* (so atomic editor saves work) and
  reload live; a broken file keeps the **last-good** set rather than blanking
  the running app. The app's own SaveFlow/SaveLayout writes intentionally
  trigger the same reload + wake-up.
- **Keychain / secrets.** Tokens go through the OS keychain via
  `github.NewKeychainStore()`. `HIVE_GITHUB_TOKEN` is a read-only headless
  override; `HIVE_GITHUB_CLIENT_ID` overrides the device-flow client id. Never
  log or persist tokens elsewhere.

## Environment variables

Defined in `internal/desktop/desktop.go` unless noted:

| Var | Purpose |
| --- | --- |
| `HIVE_DESKTOP_MOCK` | Select a deterministic offline backend (`feed`/`pipeline`/`action-smoke`/`onboarding`) |
| `HIVE_DESKTOP_CONFIG` | Override the config root (holds `flows/` + `actions.yml`) |
| `HIVE_DESKTOP_FLOWS` | Override just the flows directory |
| `HIVE_DESKTOP_ACTIONS` | Override the `actions.yml` path |
| `HIVE_DATA_DIR` | App-local state root (shared with the CLI convention) |
| `HIVE_GITHUB_TOKEN` | Read-only headless auth override |
| `HIVE_GITHUB_CLIENT_ID` | Override the device-flow OAuth client id |
| `WAILS_SERVER_PORT` | Server-build port (default 8080) |
| `WAILS_VITE_PORT` | Dev Vite port (auto-picked free port otherwise) |
