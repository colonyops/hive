# Quick terminal launchers are session-scoped

- **Status:** accepted
- **Date:** 2026-08-05

## Context

A launcher (ADR launchers-are-their-own-list-in-actions-yml) is a bindable
command that opens the pop-up terminal into a program. With no terminal to
resolve, the pop-up falls back to the user's home directory
(ADR ephemeral-popup-terminals). That is right for a bare shell and wrong for a
launcher like `lazygit`, whose command was written against a checkout and exits
at once in `$HOME`.

## Decision

**A launcher with no configured `cwd` runs where a terminal is, or not at
all.** A launcher that pins a `cwd` runs there and is reachable from anywhere.

The core enforces it. `PopupTerminalsService.applyLauncher` refuses a launcher
with no `cwd` and no session slug (`KindInvalid`) and drops any caller-supplied
`Dir`, so the terminal is the only answer to where it runs. Which directory that
is (the terminal's active pane) is decided by
ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is. A gone
session fails; nothing falls back to home.

`PopupLauncher.requiresSession` tells the frontend the core's answer. A launcher
that requires a session gets the `terminal-session` command context, active only
in terminal mode with a terminal attached. Outside it the chord is not
dispatched (it falls through, not swallowed) and the palette row is hidden.

## Consequences

- A launcher's reach depends on its `cwd`. The launcher editor and the docs say
  so.
- `terminal-session` is the context for any command that needs an attached
  terminal. Reuse it instead of adding a second spelling.
- The bare pop-up shell keeps the home fallback.
