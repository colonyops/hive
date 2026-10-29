# One server registry gives every client its session APIs, and servers live in the engine config

- **Status:** proposed
- **Date:** 2026-10-28

## Context

Remote sessions (#738) need the hive CLI, its TUI, and Hive Desktop to work
with the same servers. `servers:` lived in the CLI config, which Desktop does
not read, and each `hive servers` command dialed a new ssh connection for one
call. A client that shows remote sessions next to local ones needs a lasting
connection to each server and a state it can show for a server that is down.

## Decision

- **`servers:` is engine config.** It moves to `internal/config`, which both
  programs load from the same `config.yaml`. The YAML key and fields do not
  change. The ssh target and its validation live in `internal/domain/server`,
  because `config-is-data` keeps `internal/config` off `sshdial`.
- **One registry, in `internal/hive/servers`.** `servers.Registry` holds the
  local session API (the local daemon, or the engine in-process) and one
  `daemonclient.Client` for each configured server. A client gets every
  session API from it.
- **One connection for each server.** A server's client keeps one ssh
  connection for all calls and dials a new one on the next call after a drop.
  The registry does not dial until the first call or check.
- **A state for each server.** `Check` sends Hello and records `connected` or
  `offline`, with ssh's last stderr line as the reason. `incompatible` is
  reserved for the version check of #742. `connecting` means no check has
  answered yet.
- **A timeout for each call.** Unary calls to a server stop after
  `servers.CallTimeout`, so a slow server cannot block a list of local
  sessions. Streams, such as a create that clones a repository, have no
  bound.

## Consequences

- Desktop builds the registry on start and rebuilds it on a config reload.
  Its Code view does not show servers yet (#749).
- `hive servers` gives no engine to the registry, so `Local` is nil for those
  commands.
- A server's state changes only on `Check`. A failed session call does not
  mark the server offline.
