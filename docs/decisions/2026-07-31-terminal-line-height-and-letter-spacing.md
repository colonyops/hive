# Terminal line height and letter spacing are settings, and line height defaults to 1.2

- **Status:** accepted
- **Date:** 2026-07-31

## Context

At xterm's `lineHeight: 1` rows are drawn edge to edge, and a screen of
agent-TUI output reads as a wall. Comparable apps expose both cell metrics and
ship a line height above 1.

A taller line does not break box drawing on the atlas renderers. `TextureAtlas`
strokes a custom glyph across the full padded cell, and the renderer's
centring offset for the char box is cancelled by the same term in the custom
glyph's `offset.y`, so box drawing still tiles exactly at any line height. Only
ordinary text is centred in the taller cell. This replaces decision 4 of ADR
terminal-atlas-renderer where it said otherwise.

`letterSpacing` is added to the device char width and rounded, so it is whole
device pixels, and a fractional value quantises to nothing.

## Decision

1. **`terminal_line_height` and `terminal_letter_spacing` are user settings**
   under `appearance` in `settings.yaml`, applied to open panes. Line height
   offers 1 to 1.6 in tenths; letter spacing offers 0 to 3.

2. **Line height defaults to 1.2, letter spacing to 0.** Widening the cell
   costs columns that TUIs lay out against.

3. **Letter spacing's default must stay 0.** Appearance settings encode
   "nothing persisted" as the zero value (ADR desktop-configuration), and 0 is
   also a real setting here. Moving the default would make "no extra tracking"
   unselectable without a different encoding.

4. **The remembered size vote is keyed on every metric that moves a cell**
   (`cellMetrics` in `useTerminalWindows.ts`), not on the font size alone.
   Replaying a vote measured under other metrics would resize the session for
   every attached client to a grid nothing measured.

## Consequences

- Neither knob fixes the box-drawing seams in #131; the line-height ladder is a
  second axis of that checklist.
- Verification is visual. Settings ▸ Appearance carries a live preview that is
  a real `Terminal` on a real atlas renderer, because a CSS mock would be wrong
  in exactly the places these settings affect.
- Session panes, the pop-up terminal and the preview all claim their renderer
  through `lib/terminalRenderer.ts`, so the WebGL, canvas, DOM fallback exists
  once.
