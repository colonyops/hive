---
kind: added
---

**Flows can launch sessions and chats directly.** The new launch-session and launch-chat nodes start a coding session or an agent workspace chat for each item that reaches them, with no actions.yml entry. A launch-session node works in the item's own repository or in one fixed repository you pick. Each node launches once per item, so a pull request that keeps changing does not start a new session on every update. The item's detail pane now lists the chats it started, beside its sessions.
