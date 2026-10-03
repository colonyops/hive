---
kind: changed
---

**Messages are capped per topic.** `hive msg pub` keeps the newest 100 messages in each topic and deletes older ones, the same limit Hive Desktop already applied to the shared database. Set `messaging.max_messages` to change the limit, or `0` to keep every message.
