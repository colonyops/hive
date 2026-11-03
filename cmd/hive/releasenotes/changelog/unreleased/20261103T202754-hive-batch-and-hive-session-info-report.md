---
kind: fixed
---

**`hive batch` and `hive session info` report failure in their exit status.** `hive batch` exited 0 on unreadable or invalid input, and `hive session info` exited 0 outside a hive session. Both now exit 1. `hive doc messaging` no longer names a `--new` flag that does not exist.
