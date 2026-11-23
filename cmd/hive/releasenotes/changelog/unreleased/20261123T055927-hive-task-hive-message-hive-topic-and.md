---
kind: changed
---

**`hive hc`, `hive msg` and `hive todo` run the same operations Hive Desktop serves as MCP tools**, print JSON Lines when stdout is not a terminal, and take `--caller` and `--input` (JSON inline, `@file` or `-` for stdin). Breaking: `hc show` is `hc get`, `hc comment` is `hc comment create`, `hc context` is `hc context get`, `hc prune` is `hc clear` with `--statuses`, `hc create --parent` is `--parent-id` and a tree reads with `--input -`, `hc list --all` is `--status all`, `--json` is gone, `msg pub -t ... -m` is `msg create --topics ... --payload`, `msg list` reads messages and `msg topic list` lists topics, `msg topic` is `msg topic new`, `todo add` is `todo create` without `-u` and `-s`, and a `hive dt` resource of two words is two words (`hive dt chat pin put`). `hive msg read put` marks messages read; `show`, `ls`, `prune`, `pub` and `add` stay as aliases.
