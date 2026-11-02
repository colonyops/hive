# The sidebar tree is the only window list

- **Status:** point 1 superseded by [a-code-session-is-a-panel-layout-that-the-tree-mirrors-and-tmux-follows](2026-11-01-a-code-session-is-a-panel-layout-that-the-tree-mirrors-and-tmux-follows.md); points 2 to 4 stand
- **Date:** 2026-08-02

## Context

Terminal mode drew a session's windows twice: as a tab strip above the pane and
as rows in the sidebar tree, which lists every active session's windows. Every
window feature (selection, drag reordering, insertion markers, drop-edge rules)
had to be built into both.

## Decision

1. **The tree (`TerminalSidebar.vue`) is the window list. There is no tab
   strip.** A second view of one list makes every window feature cost twice.

2. **A window's controls live on its row.** Close is a hover control on the
   window row; rename is a double-click on it. Adding a window is a control on
   the session row, because a window belongs to a session, and it works on a
   session that is not attached through the slug-keyed call.

3. **A row's trailing slot is two fixed cells.** The second holds the status
   glyph, and a hover control that replaces it shares that cell; a control that
   must coexist with the status goes in the first. Fixed, so a name truncates at
   the same place whatever the row carries.

4. **The pane carries no chrome of its own.** Text size is set in Settings or
   by the zoom chords (ADR
   the-zoom-chords-step-the-terminal-text-size-instead-of-magnifying-the-webview),
   and find is reached by its shortcut from a focused pane, with no button.

## Consequences

- Window drag-and-drop has one surface, and a drop edge is always top or
  bottom.
- A tree control can act on a session that is not attached, so its failure has
  no attached session to report through and must surface on its own.
- Rename by double-click is not advertised anywhere in the tree.
