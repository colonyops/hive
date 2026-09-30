# The terminal text size is a pixel count

- **Status:** accepted
- **Date:** 2026-09-23

## Context

ADR the-zoom-chords-step-the-terminal-text-size-instead-of-magnifying-the-webview
put <kbd>⌘+</kbd>/<kbd>⌘-</kbd> on the text size and had them walk the five
presets, on the reasoning that each preset has known-good cell metrics and that
a wider range is more presets.

That ladder is four steps wide and stops at 18px. A chord whose point is
repetition stops answering after the third press, which is the complaint the
chords were bound to answer.

Nothing in the renderer needs the size to be one of five values: xterm measures
the cell from the resident face at whatever size it is given, and the atlas
rasterises per size (ADR terminal-atlas-renderer).

## Decision

**The size the app runs on is a pixel count** — stepped by two, clamped to
8–64, where the bounds are legibility at the bottom and a usable grid at the
top. The frontend deals only in pixels.

**`appearance.terminal_font_size` is an integer.** Zero means the default,
13px, like `terminal_font_weight`. Settings version 6 rewrites a name the preset
UI wrote — small 12, medium 13, large 14, xl 16, xxl 18 — to its pixel count,
and drops a value that is neither a name nor a number.

Keeping the names as permanent input was considered and rejected. The store
re-marshals the whole file on every save, so a name would survive a UI change
while the comments and layout around it did not. The 2px ladder from 13px also
steps off the named sizes on the first press, so most files would go numeric
anyway. A migration is one step in a mechanism that already exists.

**Settings ▸ Terminal ▸ Font size is a stepper** (`SettingsStepper.vue`), since
a segmented control would need a button per rung.

## Consequences

- **The names are gone.** A name in `HIVE_DESKTOP_APPEARANCE_TERMINAL_FONT_SIZE`
  now fails to parse, since the migration only rewrites the file.
- A settings file at version 6 is rejected by an older build, as with every
  settings migration.
- **The bounds are stated twice** — `settings.MinTerminalFontSizePx` /
  `MaxTerminalFontSizePx` govern the file, and the frontend constants hold the
  ladder and the stepper. The Go pair is the one that decides what persists.
- **An out-of-range size is clamped** rather than failing the load. One
  mistyped appearance field is not worth refusing to start.
- Canvas reader typography keeps its own preset names in the frontend
  (`useCanvasTypography.ts`). The terminal size diverges because a chord steps
  it; a picker-only setting has no reason to leave its names behind.
- The step is not proportional, so a press moves less at 60px than at 10px. If
  that reads as sluggish the answer is a proportional step, not named rungs.
