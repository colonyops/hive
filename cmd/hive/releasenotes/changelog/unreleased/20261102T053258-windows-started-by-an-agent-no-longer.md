---
kind: fixed
---

**tmux windows started by an agent no longer count as agents.** A process started from inside Claude Code, such as a dev server or a shell, showed as a running agent in the TUI's session tree, and on Linux servers even an idle shell did.
