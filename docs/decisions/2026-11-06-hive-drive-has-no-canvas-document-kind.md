# Hive Drive has no canvas document kind

- **Status:** accepted
- **Date:** 2026-11-06

## Context

A canvas did what markdown and html documents already do. Agents had two
write paths for the same output, block tools and `document_put`, and had to
pick one. The canvas html block also needed its own Go sanitizer and the `hv-`
class vocabulary, which Hive had to keep in step with every block an agent
wanted to draw. An `.html` document already isolates a whole page in a
sandboxed iframe.

## Decision

**A drive document is `dir`, `markdown`, `html`, or `file`.** The canvas kind,
the eight canvas MCP tools, the `/mcp/canvas` address, the `hive-canvas`
catalog entry, and the `hive://canvas/...` link and the `canvas=` link
parameter are removed. Agents write `.md` and `.html` documents through the
`hive-drive` entry at `/mcp/drive`, and links name a document as
`hive://drive/<space>/<path>`.

The html block sanitizer, the `hv-` class vocabulary, and canvas markdown
export go with it. The `.html` document wrapper (the sandboxed iframe and the
injected CSP) stays.

## Consequences

- An existing `<name>.canvas.json` stays on disk and lists as a plain `file`.
  No migration converts it, and the legacy canvas migrations are removed.
- Prune no longer keeps `canvases/` folders or `.canvas.json` files.
- This is a breaking change for agent configs. The canvas tool names, an MCP
  config that points at `/mcp/canvas`, and `hive://canvas` links stop working.
- The appearance settings `canvas_line_spacing` and `canvas_page_width` keep
  their names. They style the Drive reader for every document.
