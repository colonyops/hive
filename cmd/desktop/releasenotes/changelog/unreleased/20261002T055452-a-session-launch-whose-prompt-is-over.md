---
kind: fixed
---

A session launch whose prompt is over 12 KiB now fails before the session is created. The error names the prompt size and the limit instead of tmux's "command too long".
