# A binding that starts with the primary modifier works from a terminal pane

- **Status:** accepted
- **Date:** 2026-10-07

## Context

A focused pane owns every key a terminal can use. If only commands with a
per-command flag can reach the app from a pane, a user who binds ⌘I to Go to
Inbox gets nothing from a terminal, and Settings cannot change that.

## Decision

**Any binding whose first step uses `mod` works from a pane.** The pane checks
the key through `terminalEscapeCombo` (Command on macOS, Ctrl+Shift elsewhere)
and resolves it against the live keymap. There is no per-command opt-in.
`piercesPane` in the catalog stays for the chords the escape form cannot
carry.

**A sequence can start from a pane on the escape chord.** While it is pending,
xterm declines every key and the dispatcher steps the sequence. A key that
does not continue it is dropped, as it is outside a pane. Bare keys never
start a sequence over a pane, so typing `git` reaches the shell.

**Terminal find is the `terminal.find` command**, default ⌘F, in the
`terminal-pane` context (one of the Code view's panes has focus). A
`terminal-pane` command resolves first while that holds and is skipped
otherwise, so it shares ⌘F with `view.focus-search` without a conflict.

## Consequences

This narrows the "a sequence neither starts nor survives" rule in ADR
keybindings-are-chord-sequences-not-a-leader-key to bare-key sequences.

Where `mod` is Ctrl, a shifted binding cannot be reached from a pane, because
the escape drops the Shift.

Every emulator (the Code view's panes, the pop-up, and the chat pane) declines
the keys the dispatcher takes through one check, `appClaimsPaneKey` in
`lib/terminalKeys.ts`. It declines a bound escape chord whether or not its
command's context is active, so such a key does nothing in a pane where the
command does not apply. The pop-up and chat panes have no find bar, so ⌘F
there falls through to Focus search.
