---
kind: changed
---

**Canvases are gone from Hive Drive; agents write Markdown or HTML documents instead.** The eight `canvas_*` MCP tools, the `/mcp/canvas` address, the Hive Canvas MCP entry, and `hive://canvas/...` links are removed, along with the `canvas=` parameter on `hive://code` and `hive://chats` links. Point agent configs at `/mcp/drive` (Hive Drive), have agents use `document_put`, and link to documents with `hive://drive/<space>/<path>`. An existing `.canvas.json` file stays on disk and shows in Drive as a plain file.
