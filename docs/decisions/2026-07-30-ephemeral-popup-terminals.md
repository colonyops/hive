# Ephemeral pop-up terminals this process owns, beside the tmux ones it does not

- **Status:** proposed; point 2 amended by [ADR ptyterm-terminals-are-caller-addressed](2026-08-03-ptyterm-terminals-are-caller-addressed.md) — a caller may now supply the terminal's id rather than always receiving a minted one; [ADR agent-workspace-sessions-are-tmux-sessions](2026-08-03-agent-workspace-sessions-are-tmux-sessions.md) moved agent workspace sessions off this backend onto tmux, so "the pop-up" below is now the only caller; point 3's directory order is amended by [ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is](2026-08-29-a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is.md) — a slug resolves to its terminal's active pane
- **Date:** 2026-07-30

## Context

Every terminal in this app goes through tmux. `tmuxcc` attaches `tmux -C` to the
session a hive slug names, and that is right for what it serves: an agent's
session has to outlive the app, be re-renderable from `capture-pane` on
re-attach, and be shareable with whatever else the user has attached to it.

None of that applies to the terminal you want for thirty seconds. Opening
`lazygit`, adding a worktree, running one command against a checkout and reading
its output — these want to appear over what is already on screen and be gone
afterwards. Serving them through tmux would mean naming a session that has no
name to give, paying a multiplexer's costs for a shell nobody will re-attach to,
and inheriting tmux's terms: availability that depends on a discovered binary
(ADR tmux-discovery), a size that is a negotiation this app is only one voice in (ADR
0036), and a slug that must equal the tmux session name in both products (ADR
0040).

## Decision

1. **`internal/app/ptyterm` runs ephemeral terminals, and it is a sibling of
   `tmuxcc` rather than a layer under or over it.** It owns a PTY and the
   process on the far end of it, a byte ring per terminal for replay, and one
   coalescing subscriber. There is no shared interface across the two backends
   and there should not be one: they answer different questions, and the seam
   would have to be designed around whichever answer is being forced to fit.

2. **A terminal is addressed by an id this process mints, not by a slug.**
   Nothing about it is durable — it has no name outside the run, nothing else
   can attach to it, and it dies with the app. That is what "ephemeral" buys:
   there is no session registry to keep in step with hive, and no lifecycle to
   reconcile after a crash.

3. **A launch is a directory and a command line.** The directory is resolved
   in order — a hive session's checkout, an explicit path, then the user's home
   — so a caller with a session in hand does not have to know where it lives and
   one with neither still lands somewhere sensible. The command is a *shell
   command line*, not an argv: it runs through a login shell, so a user's
   aliases, functions and their own PATH are what resolve it (ADR subprocess-environment). Empty
   opens an interactive shell, which is the whole of what the UI asks for today.
   Launchers — a named `lazygit` button, a per-repo command — are that same
   spec with a config surface in front of it, and are deliberately not built
   yet.

4. **`/api/terminal/popup/…` is its own path space under the terminal prefix.**
   It sits there so the bearer token and the CORS policy already guarding the
   terminal surface cover it without a second rule — a PTY is arbitrary command
   execution exactly as tmux is (ADR terminal-transport) — and it is separate rather than more
   verbs on `/api/terminal/…` because what is behind it is not a tmux session
   and is not addressed like one. Its wire is leaner for the same reason: one
   socket carries one terminal, so output is a tag and the bytes, with no
   window or pane id to parse.

5. **Getting in and out is the feature.** The toggle opens a shell immediately —
   no start affordance, no picker, nothing between the shortcut and a prompt —
   and it is allowed to fire while a terminal has focus, which almost nothing
   is, because the combo that opens it must be able to close it. Escape is never
   bound to dismiss: it belongs to whatever is running in the pane. Dismissing
   it returns focus to whatever held it before, because the pop-up is reached
   for mid-task and leaving focus on the body would cost the keystroke that
   dismissed it.

   *This said "the only shortcut" when it was written. ADR launchers-are-their-own-list-in-actions-yml added launcher
   chords for the same reason, and the command palette joined them because it is
   how you get back out of a pane — see architecture.md ▸ Pop-up terminals for
   the rule as it now stands.*

   **The panel's box is derived from the window and nothing else** — centred, at
   a fixed fraction of it, following a window resize. It cannot be dragged or
   resized, and nothing about its geometry is remembered. A stored box is a box
   that goes stale against a window that changed since, which is a bug that
   presents as "the pop-up ignored my setting"; a derived one cannot. The
   fraction is a constant until there is a setting for it. Moving and resizing
   are deliberately deferred rather than rejected — when they come back, the
   remembered box has to answer the staleness question, not re-introduce it.

   Two dismissals, and the difference is what happens to the shell. **Hiding**
   is a view change: the shell keeps running and the next toggle returns to it.
   **Exiting** — typing `exit`, or the panel's End button — takes the panel down
   with the process, because a dead pane between the user and the app would make
   the quick way in the slow way out. A stream that drops for any *other* reason
   keeps the panel up and says so, since the shell may still be alive and
   vanishing silently would read as the terminal closing itself.

## Consequences

- **Two terminal backends exist, for two different jobs.** The rule for which
  is which is the session: a terminal that belongs to a hive session is tmux's,
  and a terminal that belongs to a moment is this one's. A change to one is not
  automatically owed to the other, and neither should grow toward the other's
  job — a pop-up that needed to survive a restart would be a tmux session, and
  a session terminal that did not need to would not be one.
- **Nothing survives the process.** `App.Close` ends every open terminal, so
  whatever was running in one is gone, including work a user did not save. This
  is the trade the surface exists to make, and the reason the shell is the whole
  UI: there is no session list to imply otherwise.
- **The replay ring is not the tmux first paint.** ADR terminal-first-paint-carries-scrollback reconstructs a pane
  from tmux's own history, screen and cursor, so what it replays is always a
  coherent screen. A byte ring is the raw stream with its head cut off: a replay
  can begin inside an escape sequence and re-runs alternate-screen transitions
  the pane already made, and it costs a resident buffer per terminal. It is not
  the weaker of two options for the same job — nothing outside this process
  holds these bytes, so there is no history to ask for and no cursor to query.
  It exists so the panel can be dismissed and brought back, which is a short
  round trip, not so a screen can be reconstructed hours later.
- **Size is applied, not negotiated.** One client renders these PTYs, so ADR
  0036's vote/grant rules and the size-constraint banner belong to tmux alone.
- **What is not built.** Tabs, more than one pop-up at a time, panes, session
  sharing, a detachable daemon, and Windows (ConPTY). The manager already holds
  a set keyed by id and the API lists it, so a second terminal is a UI decision
  rather than an engine change.
