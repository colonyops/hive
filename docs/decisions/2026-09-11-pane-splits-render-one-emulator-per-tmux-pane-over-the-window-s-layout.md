# Pane splits render one emulator per tmux pane over the window's layout

- **Status:** accepted
- **Date:** 2026-09-11

## Context

Terminal mode must show a split tmux window as tmux lays it out, and let the
user split from Hive. tmux already owns everything a split needs: the layout
string names every pane's box in cells, `%layout-change` carries it on every
split, resize, zoom and kill, `split-window`, `select-pane`, `resize-pane`
and `kill-pane` are the verbs, and `%window-pane-changed` names the pane the
keyboard belongs to.

## Decision

1. **One emulator per pane, placed by tmux's layout in cells.** The control
   client (`internal/platform/tmux/control`) parses the layout into a tree,
   keeps it on `Window` with `ActivePane` and `Zoomed`, and forwards output
   from every pane of a tracked window. The frontend multiplies the layout's
   cells by the measured cell size and positions each pane absolutely in the
   window's box. tmux announces a split or kill in two notifications, with an
   active pane outside the layout in between; the controller withholds that
   window and publishes one `layout-changed` once it is complete, so every
   event is a snapshot the renderer applies whole. The window's size vote is
   computed over the window's box, not by the fit addon, which can only
   measure one pane's host.

2. **A first paint is per pane, at the pane's layout height** (the window's
   height for a zoomed pane), so the viewport-height invariant of
   ADR terminal-first-paint-carries-scrollback holds per emulator. The active
   window's visible panes paint before attach answers; a split paints its
   new pane through the reconcile its `%layout-change` triggers.

3. **Client frames name a pane, and the wire is v2.** Input and paste carry
   a pane id, because the keystrokes that follow a click into a pane leave
   while the `select-pane` is still in flight; resolving them to the active
   pane on the server would land them in the pane the user just left. A
   client on another wire version is refused at the handshake.

4. **The keyboard is tmux's active pane.** A click into a pane moves the
   active pane at once and sends `select-pane`. tmux's announcement moves DOM
   focus onto the new active pane only when the keyboard was already in that
   window, never out of a pane in another window.

5. **Dividers are tmux's borders, and a drag is `resize-pane -x/-y`.** tmux
   moves the divider on the far side of the cell it resizes, so the frontend
   names a pane in the cell before the divider and its absolute target size,
   clamped so the cell after keeps a column. tmux answers with the layout it
   settled on; nothing moves a pane locally. Requests coalesce to one in
   flight with the latest size waiting.

6. **Control routes and bindable commands.** `split`, `select` (with an
   optional direction), `close`, `foreground`, `resize` and `zoom` under
   `/api/terminal/panes/`, and the commands `terminal.split-right`,
   `terminal.split-down`, `terminal.close-pane`, `terminal.zoom-pane` and
   `terminal.focus-pane-{left,right,up,down}`. Closing a pane is gated by the
   same process-state check as closing a tab, asked of the one pane.

## Consequences

- A pooled session holds an emulator per pane, and a visited window claims a
  renderer per pane, so the renderer budget grows with the panes a user
  looks at.
- Output from every pane crosses the broker and counts against its byte
  bound, so a flood in an unwatched pane can degrade the stream. Resync
  repaints every pane.
- A zoomed pane draws over the window while its siblings keep their
  emulators current, so unzooming is a reposition, not a repaint. Zoom shows
  as a badge, because tmux's status line is not on screen.
