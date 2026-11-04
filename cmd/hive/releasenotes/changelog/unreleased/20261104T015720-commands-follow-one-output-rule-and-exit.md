---
kind: changed
---

**Commands follow one output rule and exit with a code for the kind of error.** `--json` is the only JSON switch and it prints JSON Lines. `hive doctor --format json` is now `hive doctor --json`, and `hive x assess watch --jsonl` and `hive x assess replay --jsonl` are now `--json`. `hive doctor` prints its text report to stdout. `hive config`, `hive batch`, `hive hc prune`, `hive hc context --json`, and `hive detect` print one JSON line instead of indented JSON. When a command in JSON mode fails, it prints `{"kind", "message"}` on stderr. `hive session info --json` no longer prints an error on stdout. Exit codes: 2 invalid, 3 not_found, 4 conflict, 5 unavailable, 6 unauthenticated, 1 any other error.
