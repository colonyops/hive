# Ephemeral pop-up terminals this process owns, beside the tmux ones it does not

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Session terminals go through tmux (`tmuxcc`, `internal/platform/tmux/control`),
because an agent's session must outlive the app, re-render from `capture-pane`
on re-attach, and be shareable with other tmux clients.

None of that applies to a terminal you want for thirty seconds: `lazygit`, one
command against a checkout. Serving it through tmux would mean naming a session
that has no name, paying a multiplexer's costs for a shell nobody re-attaches
to, and inheriting tmux's binary discovery and size negotiation.

## Decision

1. **`cmd/desktop/internal/app/ptyterm` runs ephemeral terminals, as a sibling
   of `tmuxcc`.** It owns a PTY and the process on the far end, a byte ring per
   terminal for replay, and one coalescing subscriber. There is no shared
   interface across the two backends.

2. **A terminal is addressed by an id, not a slug.** `Open` mints one when the
   caller supplies none, which is what the pop-up does. A caller may supply its
   own id (ADR ptyterm-terminals-are-caller-addressed). Nothing about a
   terminal is durable: nothing else can attach to it and it dies with the app.

3. **A launch is a directory and a shell command line.** The command runs
   through the user's login shell, so their aliases, functions and PATH resolve
   it (ADR subprocess-environment). Empty opens an interactive shell. The
   directory is resolved in order: a launcher's own cwd, the active pane of the
   terminal a slug names (falling back to the hive session's checkout), an
   explicit path, then the user's home (ADR
   a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is).

4. **`/api/terminal/popup/…` is its own path space under the terminal prefix.**
   The bearer token and CORS policy guarding the terminal surface cover it
   without a second rule, since a PTY is arbitrary command execution exactly as
   tmux is (ADR terminal-transport). One socket carries one terminal, so its
   wire carries no window or pane id.

5. **Getting in and out is the feature.** The toggle opens a shell at once, and
   it fires while a terminal has focus, because the combo that opens it must be
   able to close it. Escape is never bound to dismiss; it belongs to whatever
   runs in the pane. Dismissing returns focus to whatever held it before.

   The panel's box is derived from the window alone: centred, at a fixed
   fraction, following a resize. It cannot be dragged or resized, and no
   geometry is stored, because a stored box goes stale against a window that
   changed.

   **Hiding** keeps the shell running and the next toggle returns to it.
   **Exiting** (typing `exit`, or End) takes the panel down with the process. A
   stream that drops for any other reason keeps the panel up and says so, since
   the shell may still be alive.

## Consequences

- Two terminal backends exist for two jobs. A terminal that belongs to a hive
  session or an agent workspace session is tmux's (ADR
  agent-workspace-sessions-are-tmux-sessions); one that belongs to a moment is
  `ptyterm`'s. Neither should grow toward the other's job.
- Nothing survives the process. `App.Close` ends every open terminal, unsaved
  work included.
- The replay ring is not the tmux first paint (ADR
  terminal-first-paint-carries-scrollback). It is the raw stream with its head
  cut off, so a replay can begin inside an escape sequence. It exists so the
  panel can be dismissed and brought back, not to reconstruct a screen later.
- Size is applied, not negotiated: one client renders these PTYs, so tmux's
  size vote rules do not apply.
- Not built: more than one pop-up at a time, panes, sharing, Windows (ConPTY).
  The manager already holds a set keyed by id, so a second pop-up is a UI
  decision.
