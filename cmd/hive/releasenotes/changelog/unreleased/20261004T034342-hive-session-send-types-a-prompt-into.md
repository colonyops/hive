---
kind: added
---

**`hive session send`, `keys`, and `peek` drive a session's agent from outside it.** `send` types a prompt into the agent's pane, waits for the agent to finish rendering it before pressing Enter, so the Enter is not dropped, and prints the screen after. It finds the agent window itself, so it works while the shell window is focused. `keys` presses keys, such as answers to a permission prompt, and `peek` shows the agent's state, context use, and the end of its screen.
