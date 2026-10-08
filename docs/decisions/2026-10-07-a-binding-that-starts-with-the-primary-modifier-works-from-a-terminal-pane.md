# A binding that starts with the primary modifier works from a terminal pane

- **Status:** accepted
- **Date:** 2026-10-07

## Context

A focused pane owns every key a terminal can use. A command reached the app
from a pane only when the catalog marked it `escapesPane` or `piercesPane`, so
a user who bound ⌘I to Go to Inbox got nothing from a terminal, and nothing in
Settings could change that. A sequence could not start over a pane at all.
Terminal search was a hardcoded ⌘F check in xterm's key handler, outside the
keymap.

## Decision

**Any binding whose first step uses `mod` works from a pane.** The pane checks
the key through `terminalEscapeCombo` (Command on macOS, Ctrl+Shift elsewhere)
and resolves it against the live keymap. There is no per-command opt-in, and
the `escapesPane` flag is gone. `piercesPane` stays for the chords the escape
form cannot carry.

**A sequence can start from a pane on the escape chord.** While it is pending,
xterm declines every key and the dispatcher steps the sequence. A key that
does not continue it is dropped, as it is outside a pane. Bare keys never
start a sequence over a pane, so typing `git` reaches the shell.

**Terminal find is the `terminal.find` command**, default ⌘F, in the
`terminal-pane` context: one of the Code view's panes has focus. A
`terminal-pane` command resolves first while that holds and is skipped
otherwise, so it shares ⌘F with `view.focus-search` without a conflict.

## Consequences

This narrows the "a sequence neither starts nor survives" rule in ADR
keybindings-are-chord-sequences-not-a-leader-key to bare-key sequences.

Defaults on ⌘ chords that a pane kept before now reach the app from one:
⌘[ and ⌘] (back and forward), ⌘, (settings), ⌘→ (focus pane), ⌘⇧B
(report) on macOS. Where `mod` is Ctrl, a shifted binding still cannot be
reached from a pane, because the escape drops the Shift.

Every emulator declines the keys the dispatcher takes, through one check
(`appClaimsPaneKey` in `lib/terminalKeys.ts`): the Code view's panes, the
pop-up, and the chat pane. It declines a bound escape chord whether or not its
command's context is active, so such a key does nothing in a pane where the
command does not apply.

The pop-up and chat panes have no find bar, so ⌘F there falls through to
Focus search.
