---
kind: fixed
---

A session that reuses a recycled clone is now a new session: it gets its own ID, creation time, and inbox, and no longer inherits the unread messages, group, or `hc` tasks of the session before it. `hive batch` now honors `session_id` when it reuses a clone. The recycled session's ID stops resolving when its clone is reused, and an `hc` task still assigned to that ID stays assigned until you run `hive hc update <id> --unassign`.
