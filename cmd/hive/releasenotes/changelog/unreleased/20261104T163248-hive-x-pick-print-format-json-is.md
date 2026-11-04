---
kind: changed
---

**`hive x pick --print --format json` is now `hive x pick --print --json`.** `--format` keeps `id`, `name` and `path`. `--json` prints the selected session as one JSON line. `--json` without `--print` exits 2 (invalid).
