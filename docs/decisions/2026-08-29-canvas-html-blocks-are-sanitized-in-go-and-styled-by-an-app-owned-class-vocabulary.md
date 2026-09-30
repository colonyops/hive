# Canvas html blocks are sanitized in Go and styled by an app-owned class vocabulary

- **Status:** accepted; the class vocabulary is a gate no longer, per [ADR a-canvas-html-block-is-restricted-by-what-it-can-reach-not-by-how-it-looks](2026-09-11-a-canvas-html-block-is-restricted-by-what-it-can-reach-not-by-how-it-looks.md)
- **Date:** 2026-08-29

## Context

[ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry](2026-08-28-canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry.md)
shipped `markdown` and `link` blocks and deferred an html one: "rendering
agent-authored HTML in the webview is a security decision this deliberately
defers." Demo use hit the gap it left. Markdown is the only layout lever, so
anything structural — a row of stat tiles, a two-column before/after, a card
grid, a coloured callout — gets described instead of shown. That is the same
failure canvases were built to close (#232), one level up.

Two constraints shape the answer. The pane lives in the app's own webview,
next to the Wails bindings, and any local process can claim a session id and
write a block — so script that executes there is app access, not defacement.
And the app has two themes built entirely from `--color-*` tokens: markup
that names its own colours is wrong in one of them by construction, and
markup that names none is at the mercy of whatever the model invented that
call.

The markdown renderer is not available for reuse. `renderGithubMarkdown`
escapes raw HTML on purpose and is shared with untrusted GitHub issue and PR
bodies in the feed's DetailPane; loosening it to serve canvases would loosen
it for those too.

## Decision

**A third block kind, `html`, carrying its markup in the existing `Body`
field.** No new field and no migration — canvas files are JSON and the block
model is young.

**The agent owns semantics; the app owns pixels.** An html block is written
with a `hv-` class vocabulary the app defines and documents — layout
(`hv-stack`, `hv-row`, `hv-grid` with `hv-cols-2/3/4`), containers
(`hv-card`, `hv-panel`, `hv-callout`), data (`hv-stat` with
`hv-stat-value`/`hv-stat-label`, `hv-kv`), emphasis (`hv-badge`, `hv-muted`,
`hv-mono`) and tones (`hv-info`, `hv-success`, `hv-warn`, `hv-error`,
`hv-accent`). `internal/app/canvas/html.go` declares the vocabulary and the
element allowlist; `styles/canvas-html.css` maps them onto the theme tokens.
This is the Single declaration, many consumers pattern, and it carries the
bijection test that pattern implies: a class the Go side allows but the
stylesheet does not define — or the reverse — fails
`TestHTMLVocabularyIsStyledAndDocumented`, and so does one the MCP doc page
never mentions. The agent never writes a colour, a size or a `style`
attribute, so a canvas follows the theme, survives a restyle, and matches
every other canvas without the model having to remember what it did last
time.

**Sanitizing happens once, in Go, on the way out.** `canvas.SanitizeHTML` is
a deny-by-default bluemonday policy built from the same two allowlists. It
drops rather than escapes — `script`, `style`, `link`, `meta`, `iframe`,
`object`, `embed`, `form` and its controls, every `on*` handler, and the
`style` attribute — and takes the link block's URL rule (`http`, `https`,
`mailto`, nothing relative) so a block can never render a link the pane's
click handler refuses to open. `CanvasService.GetForWorkspace` is the one
seam it runs at, which is what keeps the frontend from holding a policy of
its own; `canvas.Markdown` runs it too, because an export is opened by
something with no policy at all. **The store keeps the agent's source
verbatim**, so `read_canvas` shows the agent what it wrote rather than what
survived.

**A write that would be stripped is refused instead.** `canvas.RejectedHTML`
walks the source before it is stored and names the first element, class or
attribute the policy would drop, so `put_block` fails with `grid-cols-2` in
the message. Silent stripping is the one failure mode an agent cannot
observe — it reads its source back intact and the user sees a broken layout —
and the refusal is what makes "the app owns pixels" a contract rather than a
hope. Sanitizing still runs on the read path regardless: a file edited by
hand or written by an older build never reached that check.

**No images.** A remote `img src` in the webview is a fetch to whoever the
agent names, which is a beacon; `data:` images would cost more agent context
in base64 than the picture is worth. Revisit if a real canvas needs one.

**Rejected: a sandboxed `iframe srcdoc` per block.** It isolates better than
sanitizing into the DOM, and it defeats the point of the feature — the block
would no longer inherit the app's tokens or the pane's width, and auto-height
would need measurement plumbing between the frame and the pane. A
deny-by-default policy over a small allowlist, held to it by tests over
hostile input, is the trade taken instead.

## Consequences

- Two render paths in the pane, deliberately. `.markdown-body` stays exactly
  as strict as the untrusted GitHub bodies it is shared with; `.hv-html` is
  the only scope that renders markup, and it renders only what Go already
  sanitized.
- The vocabulary is a published contract with agents in the field. Renaming a
  class breaks canvases already on disk — they were written against the old
  names and the sanitizer will strip them. Adding is free; removing is not.
- The first cut is small on purpose. What is missing should come from
  writing real canvases, not from guessing; the classes an agent reaches for
  and cannot find are the list of what to add.
- Exported markdown carries the sanitized markup inline. Most viewers render
  it, and the `hv-` names mean nothing outside the app, so an export keeps
  the structure and loses the styling.
- The threat model is otherwise unchanged from the canvas ADR: any local
  process can still put content in a pane the user reads. What this raises is
  the expressiveness of that content, not its authority.
