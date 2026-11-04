---
kind: added
---

**`hive mcp serve` serves the Hive Desktop tools to an agent over stdio**, so its MCP configuration needs no port: `claude mcp add hive-drive -- hive mcp serve --toolset drive,layout`. It finds the running app on every call, fills the `caller` argument itself, and answers `unavailable` while the app is closed.
