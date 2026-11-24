---
kind: added
---

**`hive hc`, `hive msg`, `hive todo` and `hive drive` commands take `--server <name>` (or `HIVE_SERVER`) and run on that server**, as the session commands do. A `--caller <server>:<id>` picks the server too. A server's hive must be this release or later: an older one answers `unavailable` and names both versions, and its session commands keep working.
