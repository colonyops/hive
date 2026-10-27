# The embedded emulator never answers terminal queries on a tmux pane

- **Status:** accepted
- **Date:** 2026-08-08

## Context

tmux forwards a pane's output to a control client verbatim, queries included,
after it has already answered them down the pane's own pty. xterm.js then
parses the same query and sends its own reply back as pane input, a websocket
round trip later. By then the asking program may have stopped reading: neovim's
shutdown fence gets tmux's reply, exits, and xterm.js's `ESC [?1;2c` lands on
the shell prompt as `1;2c`. A late reply cannot be made useful, so the fix
cannot be on the reply path.

## Decision

On a pane tmux owns, xterm.js is a renderer and answers nothing.
`silenceDeviceReports` (`cmd/desktop/frontend/src/lib/terminalReports.ts`)
registers parser handlers that consume every sequence whose built-in handler
replies: device attributes, device status, DECRQM, DECRQSS, and the OSC colour
queries. Terminal mode's windows and the Agents pane install it.

The rule, shared with
[pastes are tmux paste-buffer operations](2026-08-06-pastes-are-tmux-paste-buffer-operations-not-keystrokes.md):
on a tmux pane the emulator is tmux, and the only bytes the renderer may
originate are the user's (keys, mouse, focus).

A colour set still reaches xterm.js; only the query form is consumed.
Window-size reports (`CSI 14/16/18 t`) are not on the list because xterm.js
gates them behind `windowOptions`, which no terminal here enables. Enabling one
means adding it.

Pop-up terminals (`ptyterm`) are unaffected: there xterm.js is the terminal and
must keep answering.

## Consequences

- A program that queries the background colour gets no answer on a tmux pane.
  tmux cannot answer it either, so a program that wants the theme must be told
  another way.
- The list tracks xterm.js, not a spec. `terminalReports.spec.ts` asserts that
  each listed sequence is one a real `Terminal` answers, so an xterm.js upgrade
  that adds or drops a reply fails there.
