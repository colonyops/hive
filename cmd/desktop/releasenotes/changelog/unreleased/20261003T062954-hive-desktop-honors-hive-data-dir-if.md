---
kind: changed
---

**Hive Desktop honors `HIVE_DATA_DIR`.** If you moved the hive CLI's data root, the app now finds the same sessions and tasks without a second variable. It reads `HIVE_DATA_DIR` from your login shell, so a launch from the Dock sees it too. `HIVE_DESKTOP_HIVE_DATA_DIR` still wins when set.
