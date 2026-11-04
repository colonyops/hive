---
kind: changed
---

**`hive msg inbox` takes `--caller` and `hive hc list` takes `--session-id`**, so `--session` no longer means two things. `--caller` names the calling session by id or name and reads `HIVE_AGENT_SESSION` when the flag is absent, like `hive dt`. `--session-id` filters by session id and accepts `local:<id>` as well as a bare id. An id qualified with another server is now an error, not a warning, because honeycomb items live on this machine. The old `--session` flags are gone.
