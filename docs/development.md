# Development

How to build and run the repository. For what to build and where it goes, read
[`architecture.md`](architecture.md) first — it is the standing spec, and new
work is reviewed against it.

## Layout

| Component                     | Path                                                                          | Notes                                                                                    |
| ----------------------------- | ----------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| hive CLI/TUI                  | `cmd/hive/` (+ the root `main.go`)                                            | The root program; `mise run cli:dev`, `mise run cli:build`. See the root `AGENTS.md`.            |
| Desktop app (Wails v3, Vue 3) | `cmd/desktop/` with `internal/app/` and `internal/adapter/` beneath it         | The Wails shell and its native behaviour are in `cmd/desktop/README.md`                  |
| Shared packages               | `internal/`                                                                   | Config, sessions, git, `hive.db`, hc, messaging; both programs depend on them            |
| Landing page and public docs  | `docs/` (pages under `docs/docs/`)                                            | Zensical on GitHub Pages → [hivedesktop.com](https://hivedesktop.com); `mise run docs:build` |
| Development and release tools | `cmd/tools/`, `cmd/desktop/devtools/`, `cmd/desktop/devserver/`               | ADR tool, release publisher, dev GitHub proxy. None of it ships inside the app.          |

- One Go module, `github.com/colonyops/hive`, holds every program.
- Every release ships the CLI and the desktop together under one version, `0.YYYYMMDD.N`, from one `mise run release` on the maintainer's machine (`docs/distribution.md`).
- Desktop releases are signed, notarized, and uploaded to Cloudflare R2 behind a stable domain — versioned zips plus a `latest.json` manifest that drives the in-app updater and the landing-page download link. The GitHub release carries the CLI archives and every program's notes.

## Setup

You need git, [mise](https://mise.jdx.dev), and tmux. mise installs Go, Node,
and every other tool from `mise.toml`. Docker runs the desktop e2e suite, the
CLI integration tests, and the Linux desktop build.

```sh
mise trust                         # once per clone, before mise reads mise.toml
mise install                       # toolchain + git hooks (lefthook)
mise run desktop:frontend:install  # frontend deps, for the desktop app and its tests
```

`mise install` also installs the git hooks, so a fresh clone gets the quality gates with no extra step (`mise run setup` re-installs them on demand). `mise tasks` lists every gate and build task.

Installing the hooks sets this clone's `core.hooksPath` to its own `.git/hooks`, which takes precedence over a global `core.hooksPath` — global hooks will not run in this repo.

## Running it

The hive CLI:

```sh
mise run cli:dev                # hive with the dev config: mise run cli:dev -- new, mise run cli:dev -- doctor
mise run cli:start              # hive with your global config
```

`mise run cli:dev` reads `cmd/hive/dev/config.dev.yaml` and keeps its data in
`./.data` and its log in `./dev.log`, so it never touches your own sessions
under `~/.config/hive` and `~/.local/share/hive`.

Hive Desktop:

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
Run `mise run check` before you push, and `mise run ci` for every gate,
including the e2e suite that GitHub CI does not run. The root
[`AGENTS.md`](../AGENTS.md) lists what each hook runs and why.

Never bypass a hook. A failing gate is a task to finish, not a flag to add.
