# Terminal panes render through an atlas renderer, not xterm's DOM renderer

- **Status:** accepted (decision 1's *timing* — a renderer per pane at mount —
  amended by [terminal-renderer-claimed-on-activation](2026-07-30-terminal-renderer-claimed-on-activation.md), which
  claims one when a window is first shown; decision 4 superseded by
  [terminal-line-height-and-letter-spacing](2026-07-31-terminal-line-height-and-letter-spacing.md); the atlas-over-DOM
  substance stands)
- **Date:** 2026-07-29

## Context

Terminal mode (ADR terminal-transport) opened each pane on a bare `new Terminal(...)` with no
renderer addon, so panes drew through xterm's DOM renderer. Agent TUIs — Claude
Code is the reproducer — draw their frames out of box-drawing characters and
underlines, and both came out broken: seams between rows, underlines cut between
cells (#131).

The DOM renderer cannot join either, by construction:

- **`customGlyphs` is not its option.** The default is `true`, but it is read
  only by the shared `TextureAtlas`, which only the canvas and WebGL renderers
  own. The DOM renderer has no atlas, so box drawing comes from the font's own
  glyphs, drawn to the font's metrics rather than to the cell's edges.
- **Cells are not positioned; they are spaced.** The row container gets one
  fractional `letter-spacing` derived from the average advance of `W`, and the
  browser distributes it. A cell's left edge lands wherever text layout puts it.
- **Underlines are `text-decoration` on `display: inline-block` spans.** A run
  breaks into a new span whenever a glyph's natural advance differs from the
  default — which is every Nerd Font box-drawing glyph — and decorations do not
  continue across separate inline-block boxes.

The atlas renderers have the opposite construction: `device.cell.width` is
`floor(charWidth × dpr)` and `device.cell.height` is `floor(ceil(charHeight ×
dpr) × lineHeight)`, both whole device pixels, so cells tile the canvas with no
accumulated drift at any font size or device pixel ratio. `tryDrawCustomChar`
strokes box drawing to those bounds, and a single underline is stroked from the
cell's left edge to its right — so underlines join because the cells tile
exactly, not because anything overdraws. Only the curly style extends past the
edge (half a cell each side, clipped back to the cell); relying on overdraw for
the plain underline would be relying on something the atlas does not do.

Two things this does *not* fix, both verified against a build carrying it.
Underlines under an OSC 8 hyperlink still break, because xterm forces
`underlineStyle` to dashed for any cell with a `urlId` and discards the
application's own style — a hyperlink convention, not a metrics fault, and not
reachable from any `Terminal` option. Plain underlines elsewhere, including a
60-cell run, come out solid.

## Decision

1. **Every pane loads an atlas renderer: `@xterm/addon-webgl`, falling back to
   `@xterm/addon-canvas`, with the DOM renderer only as the last resort.** The
   canvas step is not decoration. Without it, any environment missing WebGL2 —
   a Linux WebKitGTK build on software rendering (ADR linux-tarball-distribution) — keeps #131
   verbatim, and the acceptance bar for that issue is every font-size preset at
   1× and 2×. Both addons use the same `TextureAtlas`, so both are the same fix
   with a different rasterizer.

2. **The renderer is loaded after `term.open(host)`, never before.** An
   unopened `Terminal` makes the addon defer its own activation to `open()` via
   `onWillOpen`, which throws a missing-context error out of `open()` instead of
   out of `loadAddon` — past the `try` the fallback depends on. Ordering it
   after `open()` is what makes the fallback reachable.

3. **A lost WebGL context claims the canvas renderer rather than dropping to
   the DOM.** `onContextLoss` fires only when the browser did not restore the
   context itself, and the addon's disposal restores the DOM renderer on its way
   out. This is also the path a session with many windows takes: one WebGL
   context per open tab, and past the browser's context limit the oldest is
   dropped, so without this the earliest panes would silently revert to the
   broken renderer.

4. **`lineHeight` and `letterSpacing` stay unset, and pinning them is not a fix
   for #131.** Both are quantised to whole device pixels by every renderer —
   `letterSpacing` through `Math.round`, cell height through `Math.floor` —
   so neither can move a cell onto a cleaner boundary, and the row height is
   already integral at the default `lineHeight` of 1. A `lineHeight` above 1
   makes things worse: it pads the glyph away from the cell edge that box
   drawing has to reach. This is recorded because pinning `lineHeight` is the
   obvious-looking fix and was the first suspect on #131.

   **Superseded by [terminal-line-height-and-letter-spacing](2026-07-31-terminal-line-height-and-letter-spacing.md).**
   Neither knob fixes #131 and both still quantise, but the last claim is
   wrong: the atlas strokes a custom glyph across the padded cell and offsets it
   by exactly what the renderer centres the char box by, so box drawing meets
   the cell edge at any line height. Both are settings now, and line height
   defaults to 1.2.

5. **Both font weights are resident before a pane opens.** xterm measures its
   cell once on `open()` and never re-measures when a face arrives later, and an
   atlas renderer caches the glyphs it rasterised — so a bold face that lands
   after first paint stays wrong until the atlas is cleared, where the DOM
   renderer would have self-healed on its next repaint. Moving to an atlas
   renderer is what makes the preload load-bearing rather than cosmetic.

## Consequences

- **Two new frontend dependencies, both pinned to the xterm 5.x line.** The
  addons reach into `Terminal._core` for seven private services, so an addon
  major must match the core major: `addon-webgl@0.18.0` and
  `addon-canvas@0.7.0` are the releases published alongside core `5.5.0`, and
  both declare `@xterm/xterm ^5.0.0`. Upgrading the core to 6.x is a separate
  change that must move all three together.
- **Panes are a canvas, not a DOM tree.** Anything that inspected rendered rows
  through the DOM — a test, a screenshot diff, an accessibility path — no longer
  can. xterm's screen-reader mode is unaffected; it has its own live region.
- **Verification is visual and unautomated.** Nothing in CI can see a seam. The
  metrics argument above is what makes the change defensible; #131 carries the
  eyeball checklist across the font-size presets and both device pixel ratios.
- The DOM renderer remains reachable and remains broken for box drawing. It is
  a degradation path, not a supported one, and it logs when it is taken.
