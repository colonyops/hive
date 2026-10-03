---
kind: fixed
---

**A CLI-only mistake in the hive config no longer breaks Hive Desktop.** An invalid keybinding or user command in `config.yaml` stopped the app from loading the file and from saving agents and repositories to it, though Hive Desktop never uses those keys. `hive doctor` still reports them.
