---
kind: added
---

`hive hc create` and `hive hc update` take `--assign-session <session-id>` to assign an item to a named session or chat, and a tree passed with `--input` accepts `assignSession` on each child. A caller outside a session can now hand out work. The `task_create` and `task_update` tools take the same `assignSession` input.
