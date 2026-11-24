---
kind: changed
---

**`hive session` commands run the session operations that the Hive MCP tools serve.** Every flag and JSON line scripts use stays the same, and `hive session tags update` and `hive session start` are new. Piped `hive ls` and `hive session list` print JSON Lines instead of the table, and a piped command prints its error as one `{"kind","message"}` JSON line on stderr. A refusal because of uncommitted work exits 4 and an unknown server exits 3, where both exited 1.
