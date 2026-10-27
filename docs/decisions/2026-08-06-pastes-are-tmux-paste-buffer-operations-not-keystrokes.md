# Pastes are tmux paste-buffer operations, not keystrokes

- **Status:** accepted
- **Date:** 2026-08-06

## Context

A terminal emulator brackets a paste in `ESC [200~` / `ESC [201~` only when the
pane's program has enabled bracketed paste (`ESC [?2004h`). Unbracketed,
xterm.js turns each newline into a carriage return and an agent's TUI submits
one message per line.

Our emulator never sees that request. A pane is attached long after its agent
started, the first paint carries the screen, the mouse mode, and the cursor but
not bracketed-paste mode
([first paint carries scrollback](2026-07-30-terminal-first-paint-carries-scrollback.md)),
and tmux does not re-send the mode to a control client. tmux does know the
mode, and `paste-buffer -p` applies it on every version at or above the 3.2
control-mode floor (`#{bracket_paste_flag}` needs 3.6).

## Decision

A paste is its own operation on the terminal transport, not bytes on the input
path. `interceptPaste` (`cmd/desktop/frontend/src/lib/terminalPaste.ts`)
catches the pane's `paste` event ahead of xterm.js and sends `PasteChunk` /
`PasteCommit` frames. On commit the server loads the text into a named tmux
buffer over stdin and runs `paste-buffer -d -p` (`control.Client.Paste`).

- **tmux decides whether to bracket.** `-p` brackets only if the pane's program
  asked, so a shell that never asked gets plain text, not a literal `[200~`.
- **The text goes through `load-buffer` on stdin, never `send-keys`.** The
  control protocol is line-oriented, and tmux expands `#{...}` inside double
  quotes, so pasted text as a command argument is an injection. Stdin also
  keeps a pasted secret out of the process table.
- **The buffer is named** so it stays out of the numbered stack that holds the
  user's own copies.
- **Commit is a separate frame** because the 4 KiB frame cap bounds one socket
  message, not one paste. The pane must get the text as one paste or tmux
  cannot bracket it.

Raw-PTY popup terminals are unaffected: the program's own `ESC [?2004h` reaches
xterm.js live.

## Consequences

- Other DEC private modes (application cursor keys, for one) are still not
  reconstructed on attach. A change that needs them should carry them in the
  first paint rather than route more operations through tmux.
- A paste no longer passes through xterm.js, so anything that would observe it
  there (local echo, a paste-time transform) has to be added on this path.
