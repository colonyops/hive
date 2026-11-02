# A Code session is a panel layout that the tree mirrors and tmux follows

- **Status:** accepted
- **Date:** 2026-11-01

## Context

Code showed one tmux window of a session at a time, and ADR
the-sidebar-tree-is-the-only-window-list removed the tab strip so that every
window feature was built once. Code now shows a session as tabs in split
panels, and a panel holds views beside windows: Drive, Diff and Tasks. A tab
strip is back, because a split needs a place per panel to say what it shows.

The prototype let the layout know its contents: a closed union of three view
kinds, a Drive-only field on the layout root, tab ids whose spelling consumers
parsed, five copies of each view's label and icon, and a composable bound to
the Code view's pool, tree and session ops. The goal is one window and panel
abstraction that can render terminals, tasks, documents and more in a session
tab, a pop-up or another window. None of that would carry to a second host.

## Decision

1. **`lib/panelLayout` is a tiling of tabbed panels that knows nothing about
   its tabs.** A tab is `{ id, kind, params? }`. The layout places, splits,
   merges, resizes and syncs tabs, and reads neither `kind` nor `params`.
   Every rule returns the same object when nothing changed, so a store can
   tell a no-op from a write.
2. **tmux's window is one tab kind, bound in `lib/windowTabs`.** A window has
   exactly one tab, with an id derived from the window id; `windowIdOf(tab)`
   is the only reader of that spelling. `reconcileWindows` makes the window
   tabs mirror the session's windows. Which windows exist is tmux's to say.
3. **Order on screen is the layout's, and tmux follows.** The tabs and the
   tree show one order, kept in the layout. A tab that moves moves its window
   in tmux to the matching position, so the numbered chords and the tree
   agree. A merge of two panels can leave tmux's order different and one move
   cannot always repair it, so the view reads windows in tab order rather than
   repairing tmux.
4. **The tree mirrors the layout; it is not a second list.** This supersedes
   point 1 of ADR the-sidebar-tree-is-the-only-window-list. Both surfaces
   render `orderedTabs(layout)`, and a select, close or drag on either is one
   layout rule, so a feature is built once and drawn twice. Points 2 to 4 of
   that ADR stand: a window's controls live on its row, the trailing slot is
   two cells, and the pane carries no style chrome.
5. **Views are declared once, in `panelViews.ts`.** Kind, label, icon, and
   which sessions offer it. The picker, the add menus, the tab row and the
   tree read the registry. A view is open once per session, so its tab id is
   its kind. A view's own state, such as the Drive scope, travels on its tab's
   `params` and goes with the tab.
6. **A view owns no close.** The host draws the close control and handles
   Escape, whether the host is a tab, an overlay or a page. A view with one
   host has no `embedded` flag.
7. **Terminals render in a floating layer the host positions, not inside a
   panel.** An xterm element is bound for its lifetime and the attach pool
   keeps every session's terminals mounted, so panel content cannot own them.
   The layout host computes one geometry and hands it to the floating layer
   through a slot; no second formula exists. A Teleport per panel was
   rejected: a DOM move blurs the focused terminal and reflows every pane in
   it.
8. **The layout composable depends on a `PanelWindowSource`, not on the Code
   view.** `useSessionPanels(source)` takes window ids, the active window,
   closed and pending flags, and select, move, render and focus.
   `useCodePanels` builds that source from the attach pool and adds what only
   Code has: new windows and agents, and the sweep that drops the layout of a
   session that is gone.
9. **A layout is persisted per session id** in localStorage. A session the
   user closed forgets its layout; one that is merely not running keeps its
   views for when it starts.
10. **Chats is the second host, and the Drive pane is gone.** An open chat is
    a layout of one window tab, its terminal, with Drive as its only view;
    `useChatPanels` builds the `PanelWindowSource` from the chat pane's state
    and keeps the layouts in a store of their own, keyed by chat id. Its
    sidebar does not mirror the tabs: a chat row is a chat, not a list. The
    right-hand Drive pane beside a chat is removed rather than kept beside the
    tabs, so `?doc` and a `canvas=` link mean one thing in both areas: the
    session's Drive tab. The title bar's right toggle keeps the Inbox preview
    and the Drive page's comments and names Drive nowhere.
11. **Agents address tabs, not a pane.** The MCP tools `open_document`,
    `open_canvas` and `close_canvas` are replaced by `open_tab` and
    `close_tab`, which name a view (`drive`, `diff`, `tasks`, `terminal`)
    and, for an open, a placement: `behind` the tab in front with an unseen
    dot, or `beside` it in a new split. Neither takes the keyboard. A
    `terminal` is a tmux window the backend creates detached, a login shell
    in the session's checkout with an optional name, or an existing window
    named by id or name. The tool takes no command. The `hive-drive` server
    is unauthenticated behind the loopback bind because it runs nothing a
    caller writes (ADR mcp-replaces-the-agent-facing-http-api); a window it
    opens runs the user's shell with no input, and `close_tab` kills a
    window of the caller's own session. A command would make the surface
    command execution, which that ADR keeps behind the terminal token, so a
    terminal with a command is a later decision, not a flag here. No old
    name is kept: an agent reads the tool list when it connects, so
    compatibility would only preserve the pane in the vocabulary.
12. **An agent can do with the layout what the user can.** `move_tab`,
    `focus_tab`, `close_panel`, `resize_split` and `turn_split` ride the same
    `session:layout` event as `open_tab` and `close_tab`, each a verb the
    host applies with the layout's own rules, so there is no second set of
    rules for agents. A tab is addressed by what it shows, never by a panel
    id, because panel ids are the frontend's. Two verbs take something from
    the user and say so: `focus_tab` takes the screen, and `close_tab` on a
    terminal kills the window. `get_layout` reads a mirror the frontend
    reports to Go; the browser stays the only owner of the layout, and Go
    never writes it. The hive CLI (`hive dt tab`, `panel`, `split`, `layout`)
    is an MCP client of the same tools, found through an endpoint file the
    desktop writes in the data directory the two programs share, so a shell
    and an MCP agent cannot drift apart.

## Consequences

- A second host reuses `lib/panelLayout`, `PanelLayoutHost`,
  `useSessionPanels` and `useSessionLayout` with its own `PanelWindowSource`
  and floating layer. It does not reuse `TerminalPaneColumn`, which is Code's
  host. Chats is the first such host.
- Adding a view is one entry in `panelViews.ts` and one branch in the Code
  host's content slot.
- Layouts the prototype saved under `hive.code.session-tabs` are not read; the
  key changed with the shape.
- `terminal.select-window-1` to `-9` count the session's windows in the order
  their tabs appear across panels, not tmux indices; a Drive, Diff or Tasks
  tab is skipped.
- Closing a Drive tab forgets where it was reading; reopening follows the
  session's own space.
