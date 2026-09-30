# Development

How to build and run the repository. For what to build and where it goes, read
[`architecture.md`](architecture.md) first — it is the standing spec, and new
work is reviewed against it.

## Layout

| Component                     | Path                                                                          | Notes                                                                                    |
| ----------------------------- | ----------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| hive CLI/TUI                  | `cmd/hive/` (+ the root `main.go`)                                            | The root program; `mise run dev`, `mise run build`. See the root `AGENTS.md`.            |
| Desktop app (Wails v3, Vue 3) | `cmd/desktop/` with `internal/app/` and `internal/adapter/` beneath it         | The Wails shell and its native behaviour are in `cmd/desktop/README.md`                  |
| Shared packages               | `internal/`                                                                   | Config, sessions, git, `hive.db`, hc, messaging; both programs depend on them            |
| Landing page and public docs  | `docs/` (pages under `docs/docs/`)                                            | Zensical on GitHub Pages → [hivedesktop.com](https://hivedesktop.com); `mise run docs:build` |
| Development and release tools | `cmd/tools/`, `cmd/desktop/devtools/`, `cmd/desktop/devserver/`               | ADR tool, release publisher, dev GitHub proxy. None of it ships inside the app.          |

- One Go module, `github.com/colonyops/hive`, holds every program.
- Desktop releases are signed, notarized, and uploaded to Cloudflare R2 behind a stable domain — versioned zips plus a `latest.json` manifest that drives the in-app updater and the landing-page download link. GitHub releases are not user-facing.

## Setup

```sh
mise trust                         # once per clone, before mise reads mise.toml
mise install                       # toolchain + git hooks (lefthook)
mise run desktop:frontend:install  # frontend deps, for the desktop app and its tests
```

`mise install` also installs the git hooks, so a fresh clone gets the quality gates with no extra step (`mise run setup` re-installs them on demand). `mise tasks` lists every gate and build task.

Installing the hooks sets this clone's `core.hooksPath` to its own `.git/hooks`, which takes precedence over a global `core.hooksPath` — global hooks will not run in this repo.

## Running it

```sh
mise run desktop:dev                       # the app, against this worktree's isolated instance
mise run desktop:dev:onboarding            # live onboarding against a separate blank instance
mise run desktop:build                     # a local app build
```

Each worktree gets its own desktop instance and generated `launch.env`, so two
checkouts never share state. `mise run desktop:dev:fresh` recreates one.
`mise run desktop:dev:onboarding` recreates a separate blank instance and keychain
namespace, then runs the real GitHub connection flow without mock providers.

## Quality gates

Every gate is a mise task, and lefthook runs the relevant ones as git hooks.

- **pre-commit** formats staged Go files and regenerates when a generator input is staged.
- **pre-push** runs `mise run check` (generated-code drift, ADRs, desktop migrations, tidy, lint, test, goreleaser config), plus the frontend unit tests when the push touches `cmd/desktop/frontend/`.
- **`mise run ci`** is the superset: everything CI runs, plus the CLI's Docker integration tests and the e2e suite that GitHub CI does not run.

Read the `check` task rather than a copy of it — it is the single definition the
hooks, CI, and the release tool all share. Details, including why each gate
sits where it does, are in [`../AGENTS.md`](../AGENTS.md).

Never bypass a hook. A failing gate is a task to finish, not a flag to add.
