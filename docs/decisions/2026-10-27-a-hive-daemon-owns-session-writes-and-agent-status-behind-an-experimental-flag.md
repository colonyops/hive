# A hive daemon owns session writes and agent status behind an experimental flag

- **Status:** proposed
- **Date:** 2026-10-27

## Context

The hive CLI, its TUI, and Hive Desktop each build their own `hive.Engine`
over one `hive.db`. Each keeps its own status tracker, so two clients debounce
the same agent separately and can disagree. Session writes race across
processes, and nothing outside the machine can ask a hive what it runs.

## Decision

With `experimental.daemon: true` in the engine config, one `hive daemon run`
per data directory owns session writes and session-view agent status. The TUI,
the CLI session commands, and Desktop's session view call it through
`sessionapi.API`; `daemonclient.Client` implements it over Connect-RPC.
`sessionapi.Local` is the same interface over the engine, used with the flag
off and inside the daemon.

- **Transport.** Unencrypted HTTP/2 on a unix socket, `<dataDir>/hived.sock`
  (or `/tmp/hive-<uid>/…` when that path is too long), mode 0600. An flock on
  `<dataDir>/hived.lock` keeps one daemon per data directory. A remote client
  runs `ssh <host> -- hive daemon dial-stdio`, which bridges to that socket.
  There is no TCP listener, TLS, or token: SSH and file modes are the access
  control.
- **One tracker.** The daemon's status loop is the only session-view
  `FetchBatch` caller. Clients read its last snapshot, so the Stage 2 debounce
  sees one poll per interval however many clients ask.
- **The client attaches.** The daemon always creates in the background and
  returns the session. The CLI or TUI attaches or switches locally, since
  only the client has the terminal.
- **Supervision.** A client spawns the daemon on a dial failure. `hive daemon
  install` adds a launchd agent or systemd user unit that runs
  `daemon run --wait-lock` and does not kill tmux servers the daemon started.
- **Metrics.** The daemon serves `/metrics` from a private Prometheus
  registry. This deviates from the `observe.Meter` rule in
  [architecture](../architecture.md#telemetry): depguard keeps the OTel SDK
  and every exporter inside Desktop telemetry, and the CLI binary may not
  import it.

With the flag off, every client runs on its own engine as before.

## Consequences

Known gaps with the flag on:

- TUI toasts for deleted, recycled, and corrupted sessions do not fire. The
  daemon stores those notifications, so the notifications list still shows
  them.
- Desktop's review `AgentPanes` and `hive x pick` keep in-process trackers.
- The first client to spawn the daemon fixes its environment, including the
  `PATH` agents launch with.
- `hc`, `msg`, `todo`, drive, and Desktop's other services stay in-process.
- Session tag edits stay in-process in the CLI, the TUI, and Desktop, because
  `sessionapi.API` has no tag methods. The store edits tags in one
  transaction, so they do not overwrite a daemon write.
- Two creates run one after the other, because `CreateSession` points
  service-wide writers at the caller's progress stream.
- A daemon never exits on its own while its data dir exists, so each data
  dir a client touches keeps a daemon (about 40 MB, no CPU when idle). This
  is on purpose: pipeline flows will move into the daemon and need it to
  stay up. An idle timeout would break that, so the daemon needs a smarter
  rule that decides when no work depends on it before it exits on its own.
