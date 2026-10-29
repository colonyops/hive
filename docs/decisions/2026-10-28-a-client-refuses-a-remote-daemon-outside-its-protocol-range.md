# A client refuses a remote daemon outside its protocol range

- **Status:** proposed
- **Date:** 2026-10-28

## Context

A client reaches a remote daemon over ssh
([ADR one-server-fleet-gives-every-client-its-session-apis-and-servers-live-in-the-engine-config](2026-10-28-one-server-fleet-gives-every-client-its-session-apis-and-servers-live-in-the-engine-config.md)).
The hive on the server can be any version. A daemon of another version can
answer a call with fields the client does not know, or not answer a call the
client needs. Before this decision, the client did not compare versions.

## Decision

- **Protocol version.** `Hello` carries `protocol_version`, the
  `hive.daemon.v1` protocol of the daemon's build (`wire.ProtocolVersion`). A
  build supports `wire.MinProtocolVersion` to `wire.ProtocolVersion`. A daemon
  from before the field sends 0, which no build supports.
- **Remote clients check it.** A fleet server client sends `Hello` before
  the first other call on each new connection and refuses every call while the
  protocol is out of range. The error names both hive versions. The fleet
  reports the server as `incompatible`. A local daemon is not checked, because
  it is the binary that spawned it or a unit the user installed.
- **When to raise it.** Raise `ProtocolVersion` for a change that breaks a
  client or a daemon of the previous version, such as a removed RPC or a
  changed meaning of a field. A new field or RPC that an older peer can ignore
  does not raise it.
- **Environment.** `hive daemon run` sets `PATH` from the login shell
  (`execenv`), so a daemon that `dial-stdio` starts gives its agents the
  login `PATH` of the server, not the bare `PATH` of an ssh command.
- **Diagnosis.** `Health` reports whether an OS service unit started the
  daemon (`supervised`, from `--wait-lock`) and where it finds each agent
  profile's command. `hive servers probe` shows a missing agent and an
  unsupervised daemon.

## Consequences

- The client and the server need builds with overlapping protocol ranges.
  Users of remote servers upgrade both sides together until a release raises
  `MinProtocolVersion` deliberately.
- `supervised` is true only for a daemon started with `--wait-lock`, which
  only the OS service units pass.
