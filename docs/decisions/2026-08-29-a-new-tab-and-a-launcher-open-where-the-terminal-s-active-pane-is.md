# A new tab and a launcher open where the terminal's active pane is

- **Status:** accepted
- **Date:** 2026-08-29

## Context

A new tab (`terminal.new-window`, the `+` on a session's row) and a launcher
with no `cwd` opened in the session's start directory, not where its prompt had
gone since. On the scratch terminal, whose directory is `$HOME`
(ADR the-scratch-terminal-is-a-tmux-session-the-desktop-owns), every tab opened
in home. Launchers resolved the directory through hive's session listing, which
does not contain the scratch terminal or a pinned agent chat
(ADR a-pinned-agent-chat-is-attached-by-the-code-view-as-an-ordinary-tmux-slug),
so a launcher on those rows could not run.

## Decision

**The directory is the active pane's, and tmux answers.** Both `new-window`
paths in `internal/platform/tmux/control` pass `-c "#{pane_current_path}"`, and
`TerminalsService.WorkingDirectory` is the core's answer to "where is this
terminal" for anything else, such as a launcher with no `cwd`. Asking tmux
covers every row: a hive session, the scratch terminal, and a pinned chat are
all tmux sessions.

This replaces only which directory a slug resolves to in ADR
quick-terminal-launchers-are-session-scoped. A launcher with no `cwd` still runs
in a terminal or not at all, and `PopupLauncher.RequiresSession` is still what
the palette and the keymap gate on.

**A terminal that is not running falls back to the session's checkout.**
`WorkingDirectory` answers `KindNotFound` for a slug tmux is not holding, and
`PopupTerminalsService.terminalDir` falls back to `SessionDirectory`. A
stopped scratch terminal then reports that no such session exists, which is
true.

## Consequences

- A tab opens where the last one was, including in hive sessions after a `cd`.
- A launcher's `cwd` now also pins it against the moving pane directory.
- `WorkingDirectory` is read when asked, not swept with the window listing,
  because a pane's directory changes with nothing announced.
- A pane whose directory was deleted expands to a missing path; tmux falls back
  to home rather than failing.
