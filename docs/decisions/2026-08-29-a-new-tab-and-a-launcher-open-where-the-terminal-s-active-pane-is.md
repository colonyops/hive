# A new tab and a launcher open where the terminal's active pane is

- **Status:** proposed
- **Date:** 2026-08-29

## Context

Two surfaces answered "where does this open?" with the session's start
directory, and both were wrong in the same place.

`terminal.new-window` and the `+` on a session's row end in tmux `new-window`.
tmux resolves an unset start-directory to `session_path`, and the one-shot for
an unattached slug spelled that out to keep a command client from opening in
the app's own working directory. So a tab always opened where the session was
created, never where its prompt had gone since. On the scratch terminal —
whose session directory is the user's home (ADR the-scratch-terminal-is-a-tmux-session-the-desktop-owns) — every tab
opened in `$HOME` however far into a checkout the last one had been taken.

A launcher with no `cwd` resolved through `SessionsService.SessionDirectory`,
which reads hive's session listing (ADR quick-terminal-launchers-are-session-scoped). Three of the slugs the Code
view attaches are not in that listing: the scratch terminal, and a pinned agent
chat under its `agentws-*` name (ADR a-pinned-agent-chat-is-attached-by-the-code-view-as-an-ordinary-tmux-slug). The frontend gate was the route's slug, so
the chord fired on those rows and the core refused it as a session that does not
exist. `lazygit` on the scratch terminal was a launch that could not work.

## Decision

**The directory is the active pane's, and tmux is what answers.**
`#{pane_current_path}` replaces `#{session_path}` in both `new-window` paths,
and `TerminalsService.WorkingDirectory` is the core's answer to "where is this
terminal" for anything else that needs one — today, a launcher with no `cwd`.

Asking tmux rather than hive is what makes one rule cover every row in the tree.
A hive session, the scratch terminal and a pinned chat are all tmux sessions,
and a pane that has been `cd`'d somewhere is where its user is whichever kind it
is. The session record is a second answer to the same question, available for
one of the three.

This supersedes the part of ADR quick-terminal-launchers-are-session-scoped that made the checkout the
answer. What that ADR decided still holds and is the reason this one is small:
a launcher with no `cwd` runs in a terminal or not at all, `applyLauncher` still
drops the caller's `Dir` beside a slug, and `PopupLauncher.RequiresSession` is
still what the palette and the keymap gate on. Only *which* directory a slug
resolves to has changed, and with it the set of slugs that resolve at all.

**A terminal that is not running falls back to the session's checkout.**
`WorkingDirectory` answers `KindNotFound` for a slug tmux is not holding — there
is no pane to read, and the terminal domain has nothing else to offer — and
`PopupTerminalsService` is where the fallback lives, because it is the caller
that needs somewhere to open. That keeps a launcher working from a row whose
session has not been started yet, and leaves a stopped scratch terminal
reporting that there is no session by that name, which is true.

**The frontend is unchanged.** `terminal-session` already meant "terminal mode
with a slug in the route", which the scratch terminal and a pinned chat both
satisfy; only the core refused them. A gate that had been re-derived in `App.vue`
would have needed a second edit here, which is the argument ADR
quick-terminal-launchers-are-session-scoped already made for putting it in the core.

## Consequences

- **A tab is where the last one was, not where the session began.** This is what
  every terminal emulator does and what the scratch terminal needed, and it is
  a behaviour change for hive sessions too: a tab opened after a `cd` deep into
  a checkout now starts there. The session's own directory is still reachable —
  it is one `cd` away, and it is what the first tab opened in.
- **`cwd` in a launcher now pins against a moving target.** It was already the
  field that decided a launcher's reach; it is now also what stops one from
  following the pane. `launchers.md` and the editor say so.
- **A stale pane path is tmux's problem, not ours.** A pane whose directory has
  been deleted expands to a path that no longer exists; tmux's own spawn falls
  back to the home directory rather than failing the command, so a new tab still
  opens.
- **The answer is read at the moment it is asked.** `WorkingDirectory` is a
  one-shot, not swept with the window listing: a pane's directory changes with
  nothing announced, the same reason `WindowForeground` asks at the moment of a
  close (ADR closing-a-terminal-tab-is-guarded-by-process-state-not-by-pane-output).
- **Nothing new is on the wire.** `WorkingDirectory` is called from inside the
  core, so there is no HTTP or MCP surface to add and no second way for a caller
  to name a directory a session-scoped launcher opens in.
