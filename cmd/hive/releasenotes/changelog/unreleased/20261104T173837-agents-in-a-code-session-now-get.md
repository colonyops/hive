---
kind: changed
---

**Agents in a Code session get `HIVE_AGENT_SESSION`, and hive commands read it before the working directory** to know which session is calling. A shell in one session's terminal that changes into another session's checkout still acts as the first session.
