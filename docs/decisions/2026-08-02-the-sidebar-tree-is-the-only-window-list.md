# The sidebar tree is the only window list

- **Status:** accepted
- **Date:** 2026-08-02

## Context

Terminal mode drew a session's windows twice: as tabs in a strip above the pane,
and as rows in the sidebar tree. The strip came first; the tree arrived with
"Always show windows", which lists every active session's windows, not just the
attached one's. From then on the two were the same list rendered twice, and
every window feature had to be built into both — selection, drag reordering,
insertion markers, and a pair of drop-edge rules, one horizontal and one
vertical.

The strip also carried the chrome that had nowhere else to go: a `+`, a find
button, and an overflow menu whose only contents were a text-size ladder that
Settings ▸ Appearance ▸ Terminal already offers as presets.

## Decision

1. **The tree is the window list, and the strip is deleted.** Not hidden or
   made optional — a second view of one list is what made every window feature
   cost twice.

2. **A window's controls live on its row.** Close is a hover control on the
   window row; rename is a double-click on it, the gesture the tab had. Adding
   a window is a hover control on the *session* row, because a window belongs to
   a session rather than to whatever is on screen — which is also why it works
   on a session that is not attached, through the slug-keyed call rather than a
   control client the session does not yet have.

3. **A row's trailing slot is two fixed cells.** The second is where the status
   glyph sits, and the control that replaces it on hover shares that cell; a
   control that has to coexist with the status goes in the first. Fixed so a
   name truncates at the same place whatever its row is carrying, and shared so
   a close button lands where the eye already is rather than beside it.

4. **Text size is Settings only.** The overflow menu it lived in went with the
   strip, and the ladder helpers (`stepTerminalFontSize`, `resetTerminalFontSize`,
   `terminalFontSizeState`) are deleted with it — Settings uses the preset
   picker, not the nudge ladder.

   **Superseded by ADR the-zoom-chords-step-the-terminal-text-size-instead-of-magnifying-the-webview.**
   The ladder is back as a keybinding over the same presets. The pane still
   carries no control of its own.

5. **Find keeps its keyboard path and loses its button.** The find bar is
   unchanged; only the way in from the chrome is gone. This is the one capability
   that got narrower rather than moving, and it is deliberate: the shortcut
   reaches a focused pane, which is where a find is started from.

## Consequences

- **Window drag-and-drop has one surface again.** The `WindowSurface`
  discriminator, the per-surface marks, and the horizontal drop-edge rule are
  gone; a drop edge is now always top-or-bottom. The focus-restoring step on
  drag end went too — it existed because a drag ate the click the *strip* handed
  focus back to the pane on.
- **A tree control can act on a session that is not attached**, so its failure
  has no session to report through. `treeError` backs the same error strip the
  attached session's own failures use.
- **Rename stays undiscoverable.** Double-click was the tab's gesture and it is
  the row's now; nothing in the tree advertises it. Worth revisiting when the
  window row gets a menu of its own rather than only a configured-actions one.
- **The tree is a wider surface than the strip was.** It lists every active
  session's windows, so a control on a row is offered for sessions the strip
  never spoke for — which is the point, and is why the add path had to work
  without a control client.
