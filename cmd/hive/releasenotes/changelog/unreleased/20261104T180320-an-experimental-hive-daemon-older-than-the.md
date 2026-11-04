---
kind: fixed
---

**An experimental hive daemon older than the CLI is restarted.** The daemon never exits on its own, so it kept serving old code after an update. The first call of a newer `hive` replaces an older release daemon with its own binary. Agents keep running. If a service unit still starts an old binary, `hive` logs a warning: run `hive daemon install` to rewrite the unit.
