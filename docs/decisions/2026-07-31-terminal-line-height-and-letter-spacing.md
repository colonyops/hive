# Terminal line height and letter spacing are settings, and line height defaults to 1.2

- **Status:** accepted (supersedes [terminal-atlas-renderer](2026-07-29-terminal-atlas-renderer.md)
  decision 4)
- **Date:** 2026-07-31

## Context

ADR terminal-typography-is-configurable made family, size and weight configurable but left the cell metrics
alone: panes ran at xterm's `lineHeight: 1` and `letterSpacing: 0`. Rows drawn
edge to edge are the densest a terminal can be, and a screenful of agent-TUI
output reads as a wall — which is the gap against comparable apps, all of which
expose both knobs and ship a line height above 1.

ADR terminal-atlas-renderer decision 4 recorded that pinning either was not a fix for the
box-drawing seams in #131, which is true, and then went further: that a
`lineHeight` above 1 "pads the glyph away from the cell edge box drawing has to
reach". That second claim does not hold for the atlas renderers, and it is what
had kept the setting off the table.

The atlas handles `lineHeight !== 1` explicitly, in a matched pair:

- `TextureAtlas` strokes a custom glyph across the **full padded cell** —
  `tryDrawCustomChar(..., deviceCellWidth, deviceCellHeight, ...)` — not across
  the char box.
- The renderer centres the char box in the padded cell
  (`device.char.top = round((cell.height - char.height) / 2)`), and the atlas
  adds *the same* term to a custom glyph's `offset.y`. Placement is
  `-offset.y + char.top`, so for box drawing the two cancel to zero and the
  glyph is laid down at the cell's top edge at full cell height.

Box drawing therefore still tiles exactly at any line height; only ordinary text
is centred in the taller cell, which is the intent. The DOM renderer has no such
handling, but it is already a degradation path (ADR terminal-atlas-renderer consequence 4).

`letterSpacing` is a different shape than it looks. It is added to the **device**
char width and rounded — `cell.width = char.width + Math.round(letterSpacing)` —
so it is whole device pixels, not CSS pixels or points, and a fractional setting
quantises to nothing.

## Decision

1. **`lineHeight` and `letterSpacing` are user settings**, persisted in
   `settings.yaml` and applied to open panes, alongside the ADR terminal-typography-is-configurable typography
   set. Line height offers 1 to 1.6 in tenths; letter spacing offers 0 to 3.

2. **Line height defaults to 1.2, letter spacing to 0.** Line height is the knob
   that changes how the pane reads, and 1 is the wrong end of its range to
   anchor on. Letter spacing stays at 0 because its step is half a CSS pixel on
   a Retina display — real, but a refinement rather than a default — and because
   widening the cell costs columns that TUIs lay out against.

3. **Letter spacing's default must stay 0.** Appearance settings encode "nothing
   persisted" as the zero value (ADR desktop-configuration), and 0 is also a meaningful setting
   here. The two coincide only while the default is 0; moving it would make "no
   extra tracking" unselectable, and would need a different encoding first.

4. **The remembered size vote is keyed on every metric that moves a cell**, not
   on the font size preset alone. The vote is a request tmux obeys, so replaying
   one measured under different cell metrics would resize the session — and every
   other client attached to it — to a grid nothing had measured.

## Consequences

- **Existing installs get taller rows on upgrade.** Nothing persisted reads as
  the new default rather than as 1, which is the point: a setting nobody finds is
  not a fix for the density. A pane keeps its column count and loses rows, and
  the attach vote re-runs to tell tmux so.
- **ADR terminal-atlas-renderer decision 4 is half-retired.** Its first half stands — neither knob
  fixes #131, and both quantise to whole device pixels. Its second half is
  wrong and is superseded here. The box-drawing acceptance checklist on #131
  now has a second axis: the line-height ladder, not just the size presets.
- **Verification stays visual, and Settings ▸ Appearance is where it happens.**
  Nothing in CI can see a seam, so the settings carry a live preview: a real
  `Terminal` on a real atlas renderer, drawing a box whose verticals have to
  meet across rows. A CSS mock would be wrong in exactly the two places these
  settings are for — Canvas2D's heavier rasterisation (ADR terminal-typography-is-configurable) and
  cell-stroked box drawing (ADR terminal-atlas-renderer) — so the preview is an xterm instance or
  it is worthless. The unit tests cover that the values reach every pane and the
  preview alike, not what any of them rasterise to.
- **One renderer claim serves all three surfaces.** Session panes, the pop-up
  terminal and the preview go through `lib/terminalRenderer.ts`. Three copies of
  the WebGL→canvas→DOM fallback is how one surface ends up silently on the DOM
  renderer, which is the failure ADR terminal-atlas-renderer exists to prevent.
