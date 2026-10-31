---
kind: changed
---

**Hive no longer runs `spawn` and `batch_spawn` rules.** A session always starts from the `windows` of its rule, or the default layout. A config that still sets these keys keeps loading, and hive logs a warning for each such rule. Move any commands you need into `windows`.
