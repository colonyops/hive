# The desktop executable runs the hive CLI when invoked as hive

- **Status:** proposed
- **Date:** 2026-10-07

## Context

Hive Desktop and the `hive` CLI open the same `hive.db` and `config.yaml`,
but the CLI is a separate install: Homebrew, `go install`, or a release
archive. The desktop updates itself and the CLI does not, so the two drift,
and an older program runs against a schema the newer one migrated (#509).
Users who have the desktop want one `hive`, at the desktop's version.

The command has to move with every app update. The updater replaces the
install at a fixed path. The Linux update archive must hold exactly one file
(ADR linux-tarball-distribution), so a second, bundled CLI binary has nowhere
to go there. A CLI that the app downloads from the GitHub release would not
move with the updater, and that release goes live minutes after the desktop
manifest.

## Decision

**`hive-desktop` runs the CLI when `filepath.Base(os.Args[0])` is `hive`.**
That check is the first thing in `main()`. It calls `cli.Run` with the
desktop's build info, so `hive --version` reports the app's version.

**The app installs the command as a symlink, `~/.local/bin/hive`, to its own
executable.** A symlink whose target is a `hive-desktop` executable is the
app's; anything else at the path belongs to another install and the app never
writes or removes it. Each launch re-points the app's link at the running
executable, which covers an app that moved. A dev build or a translocated
app never changes the link.

**The choice is `hive_cli.install_command` in `settings.yaml`.** It is absent
until first run or Settings ▸ Hive CLI asks, so an existing install does not
gain a command it never agreed to.

`cmd/desktop/main.go` is the one desktop file depguard lets import
`cmd/hive`. This amends `docs/architecture.md`, which said neither program
imports the other.

## Consequences

- The CLI and the desktop cannot drift for a user who has the app's command
  first on PATH. A Homebrew or `go install` copy that runs first is reported
  in Settings, not replaced.
- Every `hive` call loads the desktop binary and its native libraries. A
  `hive --version` measured 27.7 ms through the desktop executable against
  23.9 ms for the `CGO_ENABLED=0` CLI (macOS arm64, release flags). On Linux
  the command needs GTK4 and WebKitGTK at load, as the desktop does.
- The desktop binary grows by about 11 MB per architecture (57 MB against
  46 MB on arm64), because it links the TUI.
- Desktop package initialization runs on every CLI call. It must stay cheap
  and free of side effects that a CLI process would inherit.
- Release, signing, notarization, and the update archives do not change: the
  link targets the executable they already ship.
