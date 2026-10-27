# The Hive runtime rebinds on a config write instead of requiring a restart

- **Status:** accepted
- **Date:** 2026-09-20

## Context

First run writes the Hive config
(ADR hive-desktop-writes-the-hive-config-during-first-run-instead-of-requiring-a-hand-written-one).
A setup step that ends by asking the user to relaunch did not work. Rebuilding
the whole App is not an option: its long-lived services hold the app's own
state, tmux control clients included.

## Decision

`App.openHiveRuntime` opens the database and the event bus once per process.
Everything the Hive config decides is built by `hive.Engine`, and
`App.ReloadHiveRuntime` loads the config and calls `Engine.Reload`, which
builds a new service set and swaps it in atomically. The agent command map
the workspace presets read is swapped beside it. A config that fails to load
or validate changes nothing: the running services keep serving the config
they were built from.

## Consequences

- The reload covers only what `Engine.build` builds. The database pool
  (`database:`), the event bus, the services that read no config, and the
  data directory keep their startup settings and need a restart. The drive
  service is kept and handed the new config.
- A config-derived dependency must be built in `Engine.build`, or it keeps
  serving the startup config.
- A replaced service set is dropped, not closed, so nothing it builds may own
  a resource that needs closing. A new dependency that does needs a teardown.
