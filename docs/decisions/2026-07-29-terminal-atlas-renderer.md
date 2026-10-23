# Terminal panes render through an atlas renderer, not xterm's DOM renderer

- **Status:** accepted
- **Date:** 2026-07-29

## Context

Agent TUIs (Claude Code is the reproducer) draw frames from box-drawing
characters and underlines, and xterm's DOM renderer breaks both: seams between
rows and underlines cut between cells (#131). It cannot join them by
construction:

- `customGlyphs` is read only by the `TextureAtlas`, which the DOM renderer
  does not have, so box drawing comes from the font's glyphs and metrics
  rather than the cell's edges.
- Cells are spaced by one fractional `letter-spacing` on the row, not
  positioned, so a cell's left edge lands wherever text layout puts it.
- Underlines are `text-decoration` on inline-block spans, and a run breaks
  into a new span at every glyph whose advance differs (every Nerd Font
  box-drawing glyph).

The WebGL renderer sizes cells in whole device pixels, so they tile with no
drift at any font size or device pixel ratio, and it strokes box drawing and
underlines to the cell's bounds.

## Decision

1. **Every pane uses `@xterm/addon-webgl`, with the DOM renderer as the only
   fallback** (xterm 6 removed the canvas renderer; ADR
   xterm-6-drops-the-canvas-renderer-so-the-dom-renderer-is-the-only-fallback).
   A lost WebGL context disposes the addon and leaves the DOM renderer. A pane
   claims its renderer when first shown, not on mount (ADR
   terminal-renderer-claimed-on-activation).

2. **The renderer loads after `term.open(host)`, never before**
   (`cmd/desktop/frontend/src/lib/terminalRenderer.ts`). On an unopened
   `Terminal` the addon defers activation to `open()`, which throws out of
   `open()` instead of `loadAddon`, past the `try` the fallback depends on.

3. **Both font weights are resident before a pane opens.** xterm measures its
   cell once on `open()`, and an atlas caches the glyphs it rasterized, so a
   bold face that arrives later stays wrong until the atlas is cleared.

## Consequences

- `@xterm/addon-webgl` reaches into `Terminal._core` private services, so its
  release must match the core's major. Upgrading the core moves both together.
- Panes are a canvas. A test, screenshot diff or accessibility path that reads
  rendered rows through the DOM cannot. xterm's screen-reader mode has its own
  live region and is unaffected.
- Verification is visual and unautomated; nothing in CI can see a seam.
- The DOM fallback still shows #131's seams. It is a degradation path and logs
  when taken.
- Underlines under an OSC 8 hyperlink still break: xterm forces a dashed
  underline for any cell with a link, and no `Terminal` option changes that.
