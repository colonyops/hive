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
  from before the field sends 0, which this build supports.
- **Remote clients check it.** A fleet server client sends `Hello` before
  the first other call on each new connection and refuses every call while the
  protocol is out of range. The error names both hive versions. The fleet
  reports the server as `incompatible`. A local daemon is not checked, because
  it is the binary that spawned it or a unit the user installed.
- **When to raise it.** Most releases raise nothing, so a client and a server
  can run different versions. Raise `ProtocolVersion` for a change that breaks
  a client or a daemon of the previous version, such as a removed RPC or a
  changed meaning of a field. Raise `MinProtocolVersion` too when this client
  cannot work around the older daemon. Only that release forces an upgrade. A
  new field or RPC that an older peer can ignore raises neither.
- **Environment.** `hive daemon run` sets `PATH` from the login shell
  (`execenv`), so a daemon that `dial-stdio` starts gives its agents the
  login `PATH` of the server, not the bare `PATH` of an ssh command.
- **Diagnosis.** `Health` reports whether an OS service unit started the
  daemon (`supervised`, from `--wait-lock`) and where it finds each agent
  profile's command. `hive servers probe` shows a missing agent and an
  unsupervised daemon.

## Consequences

- A client and a server of different versions keep working until a release
  raises `MinProtocolVersion` or `ProtocolVersion` on purpose. That release
  needs a release note, because its users must upgrade both sides together.
- A daemon on protocol 0 does not report `supervised` or its agents, so
  `hive servers probe` does not show them for it.
- `supervised` is true only for a daemon started with `--wait-lock`, which
  only the OS service units pass.
