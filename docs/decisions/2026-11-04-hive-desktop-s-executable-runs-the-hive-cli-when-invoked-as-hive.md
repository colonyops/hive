# Hive Desktop's executable runs the hive CLI when invoked as hive

- **Status:** proposed
- **Date:** 2026-11-04

## Context

The hive CLI and Hive Desktop are separate binaries. The local daemon needs a CLI on `PATH`, and a desktop install that has no CLI cannot run it. The daemon will also host pipeline flows (goja), which only the desktop links. A symlink named `hive` that points at the desktop executable gives one install, one version, and one binary for the daemon. Measured on macOS arm64, `--version` through the desktop binary takes 33.7 ms against 30.7 ms for the CLI.

## Decision

- `main()` in `cmd/desktop` checks `filepath.Base(os.Args[0]) == "hive"` before it loads settings. On a match it calls `cli.Run` from `cmd/hive/cli` with the desktop's stamped build info and returns. No environment variable triggers it.
- `cli.Run(info)` replaces the CLI's own `version` var for `--version`, the server version check, and the scripts marker. The desktop writes the same version to the scripts marker, so the two entry points do not re-extract in turn.
- Every start of the daemon sets argv[0] to `hive`, because the binary may be hive-desktop. `daemonclient.Spawn` sets `cmd.Args[0]`. The launchd unit sets `Program` to the binary and `ProgramArguments[0]` to `hive`. The systemd unit uses `ExecStart=@<binary> hive daemon run --wait-lock`.
- The desktop starts the local daemon from its own executable. `daemon.hive_path` still overrides it.
- `cmd/desktop` may import `cmd/hive/cli` and nothing else under `cmd/hive`. depguard enforces it. The CLI still must not import the desktop.

## Consequences

- Startup of the CLI through the desktop binary costs about 3 ms on macOS.
- On Linux the desktop binary links GTK, so the CLI through it loads those libraries. A headless Linux machine still needs the standalone CLI.
- The desktop binary grows by the size of the CLI.
- A binary renamed to anything other than `hive` runs as the desktop. A tool that execs the desktop with another argv[0] must set it.
- The MCP stdio resolver still looks up `hive` on `PATH`. A later change moves it to the same rule.
