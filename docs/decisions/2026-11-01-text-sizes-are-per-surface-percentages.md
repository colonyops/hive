# Text sizes are per-surface percentages

- **Status:** accepted
- **Date:** 2026-11-01

## Context

Sizing grew per surface: a pixel count for terminals, four named presets for
documents, a hardcoded size for diffs, and nothing for the chrome, whose text
tokens were in px while its spacing was in rem. No single setting could say how
big the app is, and the zoom chords only reached terminals.

## Decision

**Four independent percent scales,** `appearance.interface_scale`,
`document_scale`, `code_scale`, and `terminal_scale`. 100% is the shipped size,
0 in the file means 100, and an out-of-range value clamps. There is no master
scale: it would make a terminal's real size the product of two numbers.

- **Interface** (75-150%) sets the root font size. Text tokens, spacing, and
  sizes around text are in rem. Hairlines, radii, and the native title bar stay
  in px.
- **Documents** (75-200%) and **Code** (75-200%) are independent of the root
  font size, so their text is sized from its own scale, never from rem.
- **Terminal** (60-500%) resolves to whole pixels, because xterm rounds its cell
  to device pixels. The UI shows only the percent.

The chords act on the surface last clicked or focused, marked in the DOM by
`data-scale-surface`, and on the interface after a click outside every surface.
They step through a preset list, not a fixed amount, and they fire while an
overlay is open, because Changes is an overlay around the diffs. The chords show no confirmation:
the text changing is the feedback.

All four sizes sit on Settings ▸ Appearance, as an exception to
ADR settings-sections-name-the-surface-they-change, because their value is seeing
them together. Settings ▸ Terminal and Settings ▸ Drive also show their own row,
bound to the same setting, because that is where a person looks for it.

Webview zoom, CSS `zoom`, and `pageZoom` are ruled out. They scale an xterm grid
as well, and WebKit changed how CSS zoom reports geometry in Safari 26.4, so
behaviour would vary by OS version.

## Consequences

- A new size in the chrome is rem or a text token. `themeCss.spec.ts` fails on a
  px text token.
- FlowsCanvas lays cards out from JS constants, so its card text stays in px.
- Drive Server keeps its own browser-local reader size.
- Markdown spacing is in rem while a Drive document's text follows Documents, so
  the Interface scale also moves spacing inside a document.
- Settings version 7 rewrites `terminal_font_size` and `canvas_font_size`. The
  old `HIVE_DESKTOP_APPEARANCE_*` overrides for them are gone.
- `terminal.text-size-*` keeps its id for existing bindings but has no default
  key. The chords moved to `view.text-size-*`.
