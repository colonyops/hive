---
kind: fixed
---

**An experimental hive daemon older than the app is restarted.** The daemon never exits on its own, so it kept serving old code after an app update. The first session call replaces an older release daemon with the `hive` binary Desktop starts. Agents keep running. If a service unit still starts an old binary, Desktop logs a warning: run `hive daemon install` to rewrite the unit.
