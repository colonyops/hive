# Both programs run on one layered hive engine

- **Status:** accepted
- **Date:** 2026-10-03
- **Supersedes:** [pure-shared-domain-packages-are-imported-directly-services-and-stores-stay-behind-the-seam](2026-10-02-pure-shared-domain-packages-are-imported-directly-services-and-stores-stay-behind-the-seam.md)
- **Amends:** [desktop-configuration](2026-07-25-desktop-configuration.md) (the hive data dir variable)

## Context

The desktop treated the shared `internal/` packages as a Bounded Context: an
Anti-Corruption Layer (`dispatch/hive_adapters.go`, `hive_hc_adapters.go`)
converted every shared type, and a depguard allowlist kept the rest out. That
made sense while hive was an external module. In one module it produced copies
instead of isolation: the agent catalog, tmux exec, SQLite open, XDG paths and
a dozen small helpers each existed twice and had drifted. The desktop also
linked about 15 CLI-only packages, because the shared config imported the
CLI's keybinding types. Nothing in a path said whether a package did I/O, read
config, or belonged to one program.

## Decision

**`internal/` is a shared kernel: the hive engine both programs run on.** Its
domain types may appear in any program signature. A program converts to a
transport shape in its adapters (`wailsui`, `mcpsrv`), not at a seam in
`app`. The ACL and its depguard allowlist are deleted.

**A package's path names its layer, and imports point down.**

| Layer | Path | Holds | depguard rule |
| --- | --- | --- | --- |
| Kit | `pkg/` | Hive-agnostic helpers | none |
| Domain | `internal/domain/` | Models, rules, enums, ports. No I/O, no config | `domain-is-pure`: no other `internal/`, no `os/exec`, `net`, `database/sql`, fsnotify |
| Platform | `internal/platform/` | Drivers for outside systems: git, tmux, SQLite, process inspection, execenv, credentials, secrets, observe | `platform-is-a-driver`: only `domain` and other `platform/*` |
| Store | `internal/store/` | `hive.db` | `store-is-persistence`: only `domain` and `platform/sqlite` |
| Config | `internal/config/` | The engine sections of `config.yaml`: load, validate, write | `config-is-data`: only `domain` |
| Engine | `internal/hive/` | One subpackage per application service, and `hive.Engine` | none beyond `shared-surface-free` |
| Programs | `cmd/hive/internal/`, `cmd/desktop/internal/` | Input, rendering, program-only features and config | Go `internal` visibility; `cli-no-desktop-deps` |

`shared-surface-free` keeps charm and Wails out of all of `internal/`.
`hive.Engine` takes the drivers as `Ports` and rebuilds the config-derived
services on `Reload`; both programs use it, so the desktop's per-adapter
`Rebind` is gone.

**Both databases open through `platform/sqlite.Open` with
`_txlock=immediate`.** Every `hive.db` transaction writes, and two hc store
transactions read before they write. Under deferred locking, the CLI and the
desktop writing there at once fail with `SQLITE_BUSY` immediately, without
waiting out `busy_timeout`. Taking the write lock at `BEGIN` costs nothing for a
transaction that writes anyway. Revisit if a read-only transaction is added.

**The desktop honors `HIVE_DATA_DIR`.** It resolves the hive data dir as
`HIVE_DESKTOP_HIVE_DATA_DIR`, then `HIVE_DATA_DIR`, then its own data dir, and
reads both variables from the login shell, because a Dock launch gets none of
the shell's variables. This amends the desktop-configuration ADR, which kept
the desktop off the CLI's variable names. A user who followed the docs already
set both to one directory and sees no change.

## Consequences

- An engine change can break both programs at once. That is the point: CI
  builds and tests both.
- A new package needs no lint edit. Its path decides which rule applies.
- Programs may still import `internal/store` directly. A
  `programs-use-services` rule waits until the CLI's KV store and `sweep`, and
  the TUI's notification store and review views, stop doing so.
- `HIVE_DESKTOP_HIVE_DATA_DIR` stays for development, where it points a
  worktree's desktop at a `hive.db` other than the shell's.
