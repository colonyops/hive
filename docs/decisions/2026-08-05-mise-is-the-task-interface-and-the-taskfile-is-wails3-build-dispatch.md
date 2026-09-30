# mise is the task interface and the Taskfile is wails3 build dispatch

- **Status:** accepted
- **Date:** 2026-08-05

## Context

The repo appeared to carry two task runners. `mise.toml` held the gates and the
development commands; `desktop/Taskfile.yml` and `desktop/build/**/Taskfile.yml`
held the build. Which one owned a given job was not derivable from either file,
and four of the Taskfile's tasks (`dev`, `dev:prepare`, `dev:fresh`,
`dev:reset`) restated mise tasks badly — the Taskfile `dev` ran bare `wails3
dev` without `launch.env`/`overrides.env` and without the `devtools run`
supervision ADR shutdown-is-signalled-and-bounded requires.

The mise task names had drifted with the same ambiguity. "frontend" appeared in
three different positions (`desktop:build:frontend`, `desktop:frontend:deps`,
`desktop:test:frontend`), `release:desktop` inverted the area/verb order every
other task used, and `desktop:generate` named the area rather than the artifact
it regenerates. Meanwhile `test`, `lint`, `fmt`, `tidy` and `vendor` carried no
area prefix at all despite covering overwhelmingly desktop-app code, so the
prefix marked nothing.

## Decision

**The Taskfile stays, and it is not ours to replace.** `wails3 build` and
`wails3 package` are `wrapTask("build")` and `wrapTask("package")` inside the
CLI — they execute the tasks of those names in `desktop/Taskfile.yml`.
`wails3 dev` runs `desktop/build/config.yml`'s `dev_mode.executes`, which are
themselves `wails3 build DEV=true`, `wails3 task common:dev:frontend` and
`wails3 task run`. The release path calls tasks directly:
`cmd/release/publish.go` runs `wails3 task darwin:package:universal` and
`scripts/build/build-linux-docker.sh` runs `wails3 task linux:build`. Removing
the Taskfile means reimplementing universal-binary lipo, `.app` bundle layout,
codesign, the notarization wrapper, nfpm deb/rpm/AppImage/AUR and `.desktop`
generation, then never using `wails3 build/dev/package` again.

It also buys nothing. go-task is vendored inside the wails3 binary already
pinned in `[tools]` (`github.com/wailsapp/task/v3`); there is no `task` entry to
remove. `desktop/build/**/Taskfile.yml` are regenerated from the CLI's embedded
`build_assets/` by `wails3 update build-assets`, so hand-editing them is
transient by construction — the one deliberate deviation is pinned by
`desktop/build/taskfile_test.go`.

**mise is therefore the only interface.** `desktop/Taskfile.yml` is trimmed to
the four tasks wails3 reaches (`build`, `package`, `run`, and `setup:docker`,
which the platform Taskfiles name in their precondition messages); the eight
unreferenced tasks are deleted. `serve` stays a mise reimplementation rather
than calling `common:run:server`, because that path regenerates bindings with
`-tags server` and would fight the committed set `check:bindings` verifies.

**Task names are area-first and most-specific-last**, and the desktop app —
this repo's primary artifact — holds the bare verbs. `build`, `dev`, `serve`,
`e2e`, `bindings`, `icons`, `release`; `frontend:build`, `frontend:install`,
`frontend:test`; `build:linux`, `build:linux:image`; `dev:prepare`,
`dev:fresh`, `dev:reset`. A second area takes a prefix when it arrives —
`web:deploy`, `server:test`. `mise.toml` is grouped into generation, gates, the
app, and tooling, in that order.

## Consequences

- A task's name no longer says which runner owns it, because mise owns all of
  them. Reaching for `wails3 task <name>` directly means reaching past the
  interface, and outside `cmd/release` and the Linux build script there is no
  reason to.
- Every documented command changed. The rename touched ~40 files; a stale
  `mise run desktop:*` in an external note or shell history now fails with an
  unknown-task error rather than doing something surprising.
- `build`, `dev` and `serve` are contestable names once `server/` or `web/`
  grow tasks. That is the trade for short names on the commands typed most; the
  prefix goes on the newcomer, not retroactively on the app.
- `desktop/build/**` remains upstream-shaped. A wails3 upgrade that rewrites it
  stays a regeneration rather than a merge, which is the property that made
  keeping the Taskfile cheap in the first place.
