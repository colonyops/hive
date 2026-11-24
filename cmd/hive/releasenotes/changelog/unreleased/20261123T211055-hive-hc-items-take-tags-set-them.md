---
kind: added
---

**`hive hc` items take tags.** Set them with `hc create --tags`, change them with `hc update --add-tag`, `--remove-tag` and `--clear-tags`, and filter with `hc list --tags`. A bulk create reads a `tags` list for each item, and the `task_*` MCP tools take the same fields. Tags follow the rules of session tags.
