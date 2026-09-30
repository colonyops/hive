# Pastes are tmux paste-buffer operations, not keystrokes

- **Status:** accepted
- **Date:** 2026-08-06

## Context

Pasting several lines into a pane submitted one message per line to the agent
running in it.

A terminal emulator wraps a paste in `ESC [200~` / `ESC [201~` only when the
program in the pane has asked for bracketed paste by emitting `ESC [?2004h`.
Unbracketed, xterm.js rewrites every newline to a carriage return, and a TUI
reads each one as Enter.

Our emulator never sees that request. A pane is attached long after its agent
started, and the first paint that reconstructs the screen is `capture-pane -e`
plus a cursor address — cells and SGR, no DEC private modes
([first paint carries scrollback](2026-07-30-terminal-first-paint-carries-scrollback.md)).
tmux does not re-send the mode to a control client either. So xterm.js is
structurally unable to know, and every re-attach and pool cycle recreates the
condition.

tmux does know: it parsed the sequence when the program emitted it. It will not
tell us on the versions we support — `#{bracket_paste_flag}` only exists from
tmux 3.6, and the control-mode floor is 3.2 — but `paste-buffer -p` has applied
the answer since well before 3.2.

## Decision

A paste is a distinct operation on the terminal transport, not bytes on the
input path. The frontend intercepts the pane's `paste` event ahead of xterm.js
and sends `PasteChunk`/`PasteCommit` frames; the server loads the committed text
into a named tmux buffer and runs `paste-buffer -d -p`.

- **tmux decides whether to bracket.** `-p` brackets if and only if the pane's
  program asked for it, so a pane running an agent gets one paste and a pane
  sitting at a shell that never asked gets plain text rather than a literal
  `[200~`.
- **The text goes over the buffer, not over `send-keys`.** The control protocol
  is line-oriented and pasted text is not, and tmux's command parser expands
  formats inside double quotes — a paste containing `#{...}` would be a command
  injection. `load-buffer` reads stdin, which has neither problem.
- **The buffer is named and stdin-fed.** A named buffer stays out of the
  numbered stack holding the user's own copies; stdin keeps the text out of the
  process table, where a pasted secret would otherwise be readable.
- **Commit is a separate frame** because the 4 KiB frame cap bounds one socket
  message and says nothing about how much a person pastes. The pane has to
  receive the text as one paste or tmux cannot bracket it.

Raw-PTY popup terminals are unaffected: the program's own `ESC [?2004h` reaches
xterm.js live, so it already brackets correctly.

## Consequences

Reconstructing the pane's other DEC private modes on attach — cursor
visibility, application cursor keys, mouse reporting — is still unsolved, and
has the same cause. `paste-buffer -p` fixes the paste case without needing them,
so nothing here forces the general answer; a change that wants one should carry
the modes in the first paint rather than route more operations through tmux.

A paste no longer round-trips through xterm.js, so anything that would have
observed it there — a local echo, a paste-time transformation — has to be added
on the new path instead.
