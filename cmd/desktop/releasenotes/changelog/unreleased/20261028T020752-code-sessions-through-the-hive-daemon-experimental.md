---
kind: added
---

**Code sessions through the hive daemon** (experimental). With `experimental.daemon: true` in the hive config, the Code view creates, renames, and deletes sessions and reads agent status through the same daemon the hive CLI and TUI use, so both apps show the same status. `daemon.hive_path` in `settings.yaml` names the `hive` binary that runs it when `hive` is not on your login shell's `PATH`.
