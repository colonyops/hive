# The zoom chords step the terminal text size instead of magnifying the webview

- **Status:** accepted
- **Date:** 2026-09-17

## Context

On macOS, Wails installs a default application menu when the app sets none, and
its View menu binds <kbd>⌘+</kbd>/<kbd>⌘-</kbd>/<kbd>⌘0</kbd> to webview
magnification. Magnification scales the rendered window rather than re-laying
out the page, so a zoomed terminal grew a horizontal scrollbar and resampled
glyphs. What a terminal can do with bigger text -- re-measure its cell, re-fit,
let tmux rewrap -- never happened.

A menu key equivalent is consumed before the webview sees the keydown, so the
frontend keymap could not bind those chords while the menu held them. Linux and
Windows get no default menu and have no such accelerators.

## Decision

**On macOS the app installs the default menu with the three zoom items
removed**, and the keymap binds those chords to `terminal.text-size-increase`,
`-decrease` and `-reset`. The items go rather than keeping a click-only path to
the magnification the chords were taken away from. No other platform installs a
menu: setting one on Linux attaches a menu bar to the window and
registers every default accelerator with GTK.

**The chords walk the existing size presets** (small to xxl) and reset to
medium. The presets stay because each one has known-good cell metrics, and
because `appearance.terminal_font_size` then keeps its format. A wider range is
more presets, not a free pixel count.

*Superseded by ADR the-terminal-text-size-is-a-pixel-count:* the
size is a pixel count now, stepped by two. Five presets were too few to keep
pressing ⌘+. A settings migration turns a saved name into pixels. The rest of this
decision stands.

The commands run in an `any-terminal` context: an attached Code session, the
Agents area, or the pop-up panel over any view. The session picker is excluded,
since a size change nobody can see is a surprise later.

## Consequences

- **Nothing scales the chrome on macOS.** Nothing outside a terminal responds to
  <kbd>⌘+</kbd>, and the View menu no longer offers zoom. If that becomes a
  complaint the answer is a UI scale setting that reflows, not magnification
  back.
- A relative step needs the persisted size as its base, so
  `stepTerminalFontSize` waits for hydration before it reads the current preset.
- This supersedes the fourth point of ADR the-sidebar-tree-is-the-only-window-list:
  text size is no longer Settings-only. The pane still carries no control of its
  own, which is what that point protected.
