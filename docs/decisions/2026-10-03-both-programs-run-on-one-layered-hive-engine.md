# Both programs run on one layered hive engine

- **Status:** accepted
- **Date:** 2026-10-03

## Context

The programs in this module share `internal/`. Treating it as a separate
bounded context behind an anti-corruption layer produced copies instead of
isolation: the agent catalog, tmux exec, SQLite open, XDG paths, and small
helpers each existed twice and drifted. Nothing in a package path said
whether it did I/O, read config, or belonged to one program.

## Decision

**`internal/` is a shared kernel: the hive engine every program runs on.** Its
domain types may appear in any program signature. A program converts to a
transport shape in its adapters (`wailsui`, `mcpsrv`, `httpapi`), not at a
seam in its core.

**A package's path names its layer, and imports point down.** depguard
(`.golangci.yml`) enforces each rule; `docs/architecture.md`, "Layers and the
dependency rule", is the full reference.

| Layer | Path | Holds | depguard rule |
| --- | --- | --- | --- |
| Kit | `pkg/` | Hive-agnostic helpers | `kit-is-hive-agnostic` |
| Domain | `internal/domain/` | Models, rules, enums, ports. No I/O, no config | `domain-is-pure` |
| Platform | `internal/platform/` | Drivers for outside systems | `platform-is-a-driver`: only `domain` and other `platform/*` |
| Store | `internal/store/` | `hive.db` | `store-is-persistence`: only `domain` and `platform/sqlite` |
| Config | `internal/config/` | The engine sections of `config.yaml`: load, validate, write | `config-is-data`: only `domain` |
| Engine | `internal/hive/` | One subpackage per application service, and `hive.Engine` | `shared-surface-free` |
| Programs | `cmd/hive/`, `cmd/desktop/`, `cmd/drive-server/` | Input, rendering, program-only features and config | `programs-use-services`, `cli-no-desktop-deps`, `desktop-no-cli-deps`, `drive-server-no-program-deps` |

`shared-surface-free` keeps charm, Wails, and every program out of all of
`internal/`. `hive.Engine` takes the drivers as `Ports` and rebuilds the
config-derived services on `Reload`. `programs-use-services` lets a program
open and migrate `hive.db` (`store/db`, `store/migrate`) but reach its state
only through an engine accessor.

**Every SQLite database opens through `platform/sqlite.Open` with
`_txlock=immediate`.** Every `hive.db` transaction writes, and some read
first. Under deferred locking, the CLI and the desktop writing at once fail
with `SQLITE_BUSY` immediately instead of waiting out `busy_timeout`. Revisit
if a read-only transaction is added.

**The desktop honors `HIVE_DATA_DIR`.** It resolves the hive data dir as
`HIVE_DESKTOP_HIVE_DATA_DIR`, then `HIVE_DATA_DIR`, then its own data dir, and
reads both from the login shell, because a Dock launch gets none of the
shell's variables.

## Consequences

- An engine change can break every program at once. CI builds and tests all
  of them.
- A new package needs no lint edit; its path decides which rule applies.
- `HIVE_DESKTOP_HIVE_DATA_DIR` stays for development, where it points a
  worktree's desktop at a `hive.db` other than the shell's.
