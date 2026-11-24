---
kind: changed
---

**`session_list` and `session_tags_update` reach every configured server, and the new Hive Sessions MCP entry creates, starts, recycles and deletes sessions.** Breaking: the tools answer the `hive session list --json` shape (`repo` is the repository name, `remote` the URL, snake_case keys), and `session_tags_update` answers the session itself. `hive dt session list` and `hive dt session tags update` are gone; use `hive session list` and `hive session tags update`.
