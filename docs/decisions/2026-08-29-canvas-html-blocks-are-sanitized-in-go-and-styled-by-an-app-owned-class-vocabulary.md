# Canvas html blocks are sanitized in Go and styled by an app-owned class vocabulary

- **Status:** accepted
- **Date:** 2026-08-29

## Context

With only `markdown` and `link` blocks, anything structural on a canvas (stat
tiles, a two-column comparison, a card grid) had to be described instead of
shown. The pane lives in the app's own webview next to the Wails bindings, and
any local process can write a block, so script that runs there is app access.
The markdown renderer cannot be reused: `renderGithubMarkdown` escapes raw HTML
on purpose and is shared with untrusted GitHub bodies in the feed.

## Decision

**A third block kind, `html`, carries its markup in the existing `Body`
field.**

**Sanitizing happens once, in Go, on the way out.** `canvas.SanitizeHTML`
(`cmd/desktop/internal/app/canvas/html.go`) is a deny-by-default bluemonday
policy. The drive service runs it when it loads a canvas for the pane, and
`canvas.Markdown` runs it for an export, so the frontend holds no policy of its
own. Links take the link block's schemes (`http`, `https`, `mailto`, nothing
relative), so a block cannot render a link the pane's click handler refuses to
open. What the allowlists admit and refuse is set by ADR
a-canvas-html-block-is-restricted-by-what-it-can-reach-not-by-how-it-looks.

**The store keeps the agent's source verbatim**, so `read_canvas` shows what the
agent wrote.

**A write that would be stripped is refused.** `canvas.RejectedHTML` names the
first element, attribute, or link the policy would drop, and the write fails
with that name. Silent stripping is the one failure an agent cannot observe.
Sanitizing still runs on read, for files edited by hand.

**The `hv-` classes are the app's defaults.** `HTMLClasses` declares them,
`web/packages/ui/src/styles/canvas-html.css` maps them onto theme tokens, and
`TestHTMLVocabularyIsStyledAndDocumented` holds the class list, the stylesheet,
and the `hive-drive.md` MCP doc in agreement.

**Blocks are sanitized into the DOM, not framed.** A sandboxed `iframe srcdoc`
per block would no longer inherit the app's tokens or the pane's width and
would need height plumbing. Whole HTML documents are framed instead (ADR
hive-drive-is-one-document-tree-under-the-context-root-and-a-canvas-is-one-document-kind-in-it).

## Consequences

- The pane has two render paths on purpose. `.markdown-body` stays as strict as
  the untrusted GitHub bodies it is shared with; `.hv-html` is the only scope
  that renders markup, and only what Go sanitized.
- Renaming an `hv-` class changes how canvases already on disk look. Adding is
  free; removing is not.
- Exported markdown carries the sanitized markup inline, so an export keeps the
  structure and loses the styling.
