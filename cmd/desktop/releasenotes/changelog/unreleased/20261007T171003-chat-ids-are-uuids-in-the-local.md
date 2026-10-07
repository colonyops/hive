---
kind: changed
---

**Chat ids are UUIDs** in the local HTTP API and the MCP tools: a chat's `id`, a canvas's `session`, and a schedule run's `sessionId` are strings now, not numbers. A script that reads or sends a chat id has to treat it as a string. A chat that was running before the update keeps working under its old number.
