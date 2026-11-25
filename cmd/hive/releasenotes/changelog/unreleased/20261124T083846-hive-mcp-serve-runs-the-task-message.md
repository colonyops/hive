---
kind: changed
---

**`hive mcp serve` runs the task, message, todo, session and Drive document tools itself, so they work while Hive Desktop is closed.** It opens hive.db on the first of those calls. Creating, starting, recycling and deleting sessions, and the desktop tools, still go to the running app.
