# The zoom chords step the terminal text size instead of magnifying the webview

- **Status:** accepted
- **Date:** 2026-09-17

## Context

On macOS, Wails installs a default application menu whose View menu binds
<kbd>⌘+</kbd>/<kbd>⌘-</kbd>/<kbd>⌘0</kbd> to webview magnification.
Magnification scales the rendered window without re-laying out the page, so
a terminal grows a horizontal scrollbar and resampled glyphs instead of
re-measuring its cell and letting tmux rewrap. A menu key equivalent is
consumed before the webview sees the keydown, so the frontend keymap cannot
bind those chords while the menu holds them.

## Decision

**On macOS the app installs the default menu with the three zoom items
removed** (`wailsui/menu_darwin.go`), and the keymap binds the chords to
`terminal.text-size-increase`, `terminal.text-size-decrease` and
`terminal.text-size-reset`. The items go rather than leaving a click-only
path to magnification. No other platform installs a menu: on Linux one
would add a menu bar and register every default accelerator with GTK.

The chords step the terminal text size and reset it to the default; the
size itself is ADR the-terminal-text-size-is-a-pixel-count.

The commands run in any terminal context: an attached Code session, the
Agents area, or the pop-up panel over any view, but not the session picker,
where a size change nobody can see is a surprise later.

## Consequences

- Nothing scales the app chrome on macOS, and the View menu has no zoom. If
  that becomes a complaint, the answer is a UI scale setting that reflows,
  not magnification.
- A relative step needs the persisted size as its base, so the step waits
  for the font settings to hydrate.
- Text size is no longer Settings-only, which amends the fourth point of
  ADR the-sidebar-tree-is-the-only-window-list. The pane still carries no
  control of its own.
