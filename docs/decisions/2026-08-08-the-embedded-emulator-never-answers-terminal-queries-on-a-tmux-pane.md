# The embedded emulator never answers terminal queries on a tmux pane

- **Status:** accepted
- **Date:** 2026-08-08

## Context

Quitting neovim in a Terminal-mode pane left `1;2c` sitting on the shell prompt,
as though it had been typed.

Those are the last four bytes of `ESC [?1;2c`, a primary device attributes
reply. Two things answer that query in this app, and they disagree — tmux
answers `ESC [?1;2;4c`, xterm.js answers `ESC [?1;2c`. The bytes on the prompt
are xterm.js's.

Both answer because tmux forwards a pane's output to a control client verbatim,
queries included; the `%output` notification carrying a program's `ESC [c`
carries the escape sequence itself, not a parsed form of it. tmux has already
replied down the pane's own pty by the time that notification is written. So the
renderer receives, parses, and answers a query that was never addressed to it,
and its answer goes back as pane input over `send-keys`.

Whether that second answer is visible depends on who is reading the pane when it
lands. Neovim asks twice — once at startup, and once 44 bytes from the end of
its output as a shutdown fence. The fence is the case that breaks: nvim waits
for the one reply it expects, gets tmux's in microseconds, restores the tty and
exits. xterm.js's copy arrives a websocket round trip later, when the shell owns
the pane again.

No amount of care on the reply path fixes this. The round trip is what makes the
answer late, and a query is only useful answered before the asking program stops
reading.

## Decision

On a pane tmux owns, xterm.js is a renderer and answers nothing.
`silenceDeviceReports` registers parser handlers that consume every sequence
whose built-in handler replies — device attributes, device status, DECRQM,
DECRQSS, and the OSC colour queries — and both tmux-backed terminals install it:
Terminal mode's windows and the Agents pane.

This is [pastes are tmux paste-buffer operations](2026-08-06-pastes-are-tmux-paste-buffer-operations-not-keystrokes.md)
read in the other direction. That ADR moved an operation *off* xterm.js because
tmux holds state xterm.js structurally cannot see; this one stops xterm.js
speaking about state tmux has already spoken for. The rule under both: on a tmux
pane the emulator is tmux, and the only bytes this renderer may originate are
the ones the user produced — keys, mouse, focus.

A colour *set* still reaches xterm.js, because it renders the result; only the
query form is consumed. Window-size reports (`CSI 14/16/18 t`) are absent from
the list rather than suppressed: xterm.js gates each behind a `windowOptions`
entry and no terminal here opts in, so enabling one is what would need to add it
back.

Pop-up terminals are unaffected. `ptyterm` gives xterm.js a real pty with
nothing in between, so there it *is* the terminal and must keep answering.

## Consequences

A program that queries the background colour gets no answer on a tmux pane.
tmux cannot answer it either — it would relay to the outer terminal, and a
control client is not one — so the answer was already only ever xterm.js's late
one, correct about the rendered colour and delivered too late to be relied on. A
program that wants the theme has to be told some other way.

The suppression list tracks xterm.js, not a spec. `terminalReports.spec.ts`
asserts against a real `Terminal` that each listed sequence is one xterm.js
actually answers, so a version bump that adds a reply, or drops one, fails there
rather than on someone's prompt.
