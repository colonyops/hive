---
kind: added
---

**Session commands on servers.** `hive new`, `hive session create`, `show`, `list`, `update`, `delete`, and `recycle` take `--server <name>` (or `HIVE_SERVER`) to act on a server under `servers:`. With servers configured, `hive session list` shows the sessions of every server in a SERVER column, and warns about a server that does not answer instead of failing.
