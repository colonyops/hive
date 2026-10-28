---
kind: added
---

**`hive daemon`** (experimental). With `experimental.daemon: true` in the config, one background daemon per data directory creates and deletes sessions and tracks agent status for the TUI, the session commands, and Hive Desktop, so every client shows the same status. A client starts the daemon when none runs; `hive daemon status`, `stop`, and `install` (a launchd agent or systemd user unit) manage it. `hive servers ls` and `hive servers probe` reach daemons on other machines over ssh, listed under `servers:` in the config.
