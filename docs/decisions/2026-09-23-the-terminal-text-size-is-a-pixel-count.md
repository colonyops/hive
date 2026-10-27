# The terminal text size is a pixel count

- **Status:** accepted
- **Date:** 2026-09-23

## Context

The zoom chords step the terminal text size
(ADR the-zoom-chords-step-the-terminal-text-size-instead-of-magnifying-the-webview).
A ladder of named presets runs out after a few presses, which defeats a chord
built for repetition. The renderer needs no fixed set of sizes: xterm
measures the cell at whatever size it is given, and the atlas rasterizes per
size (ADR terminal-atlas-renderer).

## Decision

The terminal text size is a pixel count, stepped by two and clamped to
8-64px. `appearance.terminal_font_size` is an integer, and zero means the
default, 13px. Settings version 6 rewrote the old preset names (small 12,
medium 13, large 14, xl 16, xxl 18) to pixel counts. Settings ▸ Terminal ▸
Font size is a stepper (`SettingsStepper.vue`).

## Consequences

- A name in `HIVE_DESKTOP_APPEARANCE_TERMINAL_FONT_SIZE` fails to parse; the
  migration only rewrites the file.
- The bounds are stated twice: `settings.MinTerminalFontSizePx` and
  `MaxTerminalFontSizePx` decide what persists, and
  `stores/useTerminalFont.ts` repeats them for the stepper and the chords.
- An out-of-range size is clamped rather than failing the load.
- Drive document typography keeps named presets, because no chord steps it.
- The step is not proportional. If large sizes feel sluggish, the fix is a
  proportional step, not named rungs.
