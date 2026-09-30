# The Hive runtime rebinds on a config write instead of requiring a restart

- **Status:** accepted
- **Date:** 2026-09-20

## Context

`App.openHiveRuntime` loaded the Hive config once, at construction, and injected
what it derived — the session launcher, the session manager, the message
publisher, the agent command set — by value into services built immediately
after. `docs/architecture.md` recorded the consequence: changes to that file
require a Desktop restart.

That was tolerable while the file was something the user edited elsewhere. It
is not tolerable now that first run writes it
(ADR hive-desktop-writes-the-hive-config-during-first-run-instead-of-requiring-a-hand-written-one):
a setup step that ends by asking the user to relaunch is a setup step that did
not work.

Rebuilding the whole App was not available — the services holding these values
are long-lived and hold this app's own state, tmux control clients included.

## Decision

Split `openHiveRuntime` in two. The database, the event bus and the honeycomb
store are opened once and live for the process. Everything the Hive config
decides is built by `buildHiveServices`, which can run again against the same
database and bus.

Give the three `dispatch` adapters a `Rebind` method. Each holds its
config-derived dependencies in one struct behind an `atomic.Pointer` and takes
a single snapshot per method call, so a reload mid-call cannot pair a session
service with the git executor of another config. `agentCommands` becomes an
atomic pointer read through a function, so the agent-workspace preset list sees
a reload without the service being rebuilt.

`App.ReloadHiveRuntime` loads the config and swaps. A failed load changes
nothing: the running services keep serving the config they were built from.

## Consequences

- The config first run writes takes effect in the same process, so setup never
  ends by asking for a relaunch. First run is the only writer
  (ADR hive-desktop-writes-the-hive-config-during-first-run-instead-of-requiring-a-hand-written-one),
  so it is the only path that reaches this reload.
- The reload covers exactly what `buildHiveServices` builds. The database pool
  (`database:`), the event bus, and the resolved data directory keep their
  startup settings. Every later change is a hand edit, and Settings ▸ Hive CLI
  says a hand edit needs a restart.
- Adding a config-derived dependency means putting it in `hiveServices` and in
  the matching `Rebind`, or it silently keeps serving the startup config.
- The adapters are the seam this lives in, which is the Anti-Corruption Layer's
  job already. Nothing in `app` or the vendored code changed shape.
- Nothing built by `buildHiveServices` owns a resource that needs closing, so a
  reload has no teardown. A new dependency that does would need one.
