# Pane splits render one emulator per tmux pane over the window's layout

- **Status:** accepted
- **Date:** 2026-09-11
- Builds the pane splits [ADR terminal-transport](2026-07-28-terminal-transport.md)
  and [ADR terminal-first-paint-carries-scrollback](2026-07-30-terminal-first-paint-carries-scrollback.md)
  deferred.

## Context

Terminal mode rendered one emulator per tmux window, sized to the window, and
fed it the active pane's bytes. `tmuxcc` parsed only the leading `WxH` off a
`%layout-change`, dropped output from every other pane before it reached the
broker, and addressed input by window id, resolved to the active pane on the
server. A window somebody split from another client drew one pane's cells over
a grid the size of the whole window, and there was no way to split from Hive.

tmux already owns everything a split needs: the layout string names every
pane's box in cells, `%layout-change` carries it on every split, resize, zoom
and kill, `split-window`/`select-pane`/`resize-pane`/`kill-pane` are the
verbs, and `%window-pane-changed` says which pane the keyboard belongs to.

## Decision

1. **One emulator per pane, placed by tmux's layout in cells.** `tmuxcc`
   parses the layout string into a tree, keeps it on `Window` beside a
   `Zoomed` flag, and forwards output from every pane a tracked window owns.
   The wire's window event carries `activePane`, `zoomed` and `layout`; the
   frontend multiplies the layout's cells by the cell it measured off the
   first pane it opened and positions each pane's host absolutely inside the
   window's box. tmux announces a split and a kill in two notifications, and
   between them the active pane is not one of the layout's leaves; the
   controller withholds such a window and publishes one `layout-changed` when
   the next notification completes it, so every event is a snapshot the
   renderer applies whole. The window's size vote is unchanged — the box is measured and
   tmux answers with a grid — but the arithmetic is done over the window's box
   with the fit addon's formula rather than by the fit addon, which can only
   measure its own terminal's host, now one pane's.

2. **A first paint is per pane, at the pane's height.** The paint gate was
   already keyed by pane; what changes is that every leaf of a window is a
   target, at its layout height — the window's for a zoomed pane — so the
   viewport-height invariant the first-paint ADR depends on holds per emulator.
   The active window's shown panes paint before attach answers; the panes zoom
   is hiding join the deferred pass. A split paints its new pane through the
   reconcile a `%layout-change` triggers, exactly as a new window does.

3. **Client frames name a pane, and the wire is v2.** Input and paste carry a
   pane id instead of a window id, because the keystrokes that follow a click
   into a pane go out while the `select-pane` the click caused is still in
   flight; resolving them to the window's active pane on the server would land
   them in the pane the user just left. `?v=1` is refused at the handshake.

4. **The keyboard is tmux's active pane.** A click into a pane moves the tab's
   active pane at once and sends `select-pane`; tmux's announcement — from that
   click, another client, a split, or a kill — moves DOM focus onto the new
   active pane when the keyboard was already in that window, and leaves it
   alone otherwise. Focus is never pulled out of a pane in another window.

5. **Dividers are tmux's borders, and a drag is `resize-pane -x/-y`.** The
   one-cell border between two sibling cells is the drag target. tmux resizes
   the cell it is told to and moves the divider on that cell's far side, so
   the frontend names a pane inside the cell *before* the divider and the
   absolute size it should end up at, clamped so the cell after it keeps a
   column. tmux answers with the layout it settled on, which is what redraws
   the panes; nothing moves a pane locally. Requests coalesce to one in flight
   with the latest size waiting.

6. **Six control-plane routes, six bindable commands.** `panes/split`,
   `select` (with an optional direction, tmux's own `-L/-R/-U/-D`), `close`,
   `foreground`, `resize` and `zoom` under the terminal prefix, and
   `terminal.split-right` (⌘D), `-split-down` (⌘⇧D), `-close-pane` (⌘⇧W),
   `-zoom-pane` (⌘⇧↩) escaping a focused pane like the window lifecycle,
   with `terminal.focus-pane-{left,right,up,down}` (⌘⌥arrows) piercing it,
   because the escape form cannot carry an alt chord. Closing a pane is gated
   by the same process-state check as closing a tab, asked of the one pane.

## Consequences

- **A pooled session now holds an emulator per pane rather than per window**,
  and a visited window claims an atlas renderer per pane. The renderer is
  still claimed on activation, so the context budget grows with the panes a
  user actually looks at.
- **Output from every pane crosses the broker.** A background pane that used
  to be drained now counts against the byte bound like the active one, so a
  flood in a pane nobody is looking at can degrade the stream. The resync
  path is unchanged and repaints every pane.
- **The old window-addressed frontend is refused at the handshake**, which is
  what `?v=` exists for; the frontend and binary ship together, so only a
  stale dev webview can hit it.
- A zoomed pane draws over the whole window; its siblings keep their
  emulators at their layout size and stay current, so unzooming is a
  reposition rather than a repaint. Zoom is announced with a badge and nothing
  else, because tmux's own status line is not on screen.
- Not built: swapping or breaking panes out, tmux's preset layouts, and a
  pane context menu. Each is one route and one command on the shape above.
