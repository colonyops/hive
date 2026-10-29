---
kind: added
---

**Sessions on servers in the TUI.** The session tree shows a group for each server under `servers:`, with its state, after the local sessions. Rename, tags, group, recycle, delete, and open work on a server's session; open runs over ssh, in a new tmux window when the TUI runs in tmux. The status shows each session's branch, changes, and preview from its server. Press the new-session key on a server's group to create a session there. Commands that run on this machine are refused for a server's session, and an offline server shows its ssh error without slowing the tree.
