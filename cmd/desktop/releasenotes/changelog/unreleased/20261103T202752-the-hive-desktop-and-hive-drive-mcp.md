---
kind: changed
---

**The Hive Desktop and Hive Drive MCP tools take noun-first names**: `open_tab` is `tab_open`, `write_document` is `document_put`, `list_inbox` is `inbox_list`, `execute_flow` is `flow_preview`, and so on for every tool. The argument that names the caller is `caller`, not `session`, and every profile id is `profileId`, where tools took `profile` or `flowId`. The old names are gone, so update any prompt or skill you wrote that names a tool. `document_delete`, `canvas_delete`, `profile_delete` and `schedule_delete` now answer with what they deleted, `deleted: true`, and the time.
