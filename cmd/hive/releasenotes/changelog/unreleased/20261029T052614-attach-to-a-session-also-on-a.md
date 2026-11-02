---
kind: added
---

**Attach to a session, also on a server.** `hive session attach <[server:]id>` opens a session's terminal and starts it first when its tmux session ended. A session on a server opens over ssh, in a new tmux window when you run it inside tmux. `hive new --server` attaches the same way.
