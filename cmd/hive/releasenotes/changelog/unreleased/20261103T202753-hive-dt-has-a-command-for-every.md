---
kind: changed
---

**`hive dt` has a command for every Hive Desktop tool**, such as `hive dt document put`, `hive dt inbox list --profile-id <id>` and `hive dt schedule list`, each with the tool's fields as flags or the whole input as JSON through `--input`. `hive dt layout` is now `hive dt layout get`, `hive dt tab close terminal` takes `--window`, `--session` is `--caller`, and `--url` is `--desktop-url`. A refused call exits with a status for its kind: 2 for invalid, 3 for not found, 4 for a conflict, 5 for unavailable, 6 for unauthenticated, and 1 otherwise.
