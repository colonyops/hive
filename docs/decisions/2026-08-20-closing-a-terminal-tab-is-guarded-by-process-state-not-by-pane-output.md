# Closing a terminal tab is guarded by process state, not by pane output

- **Status:** accepted
- **Date:** 2026-08-20

## Context

Closing a tab killed its tmux window and everything in it on the click, from
both paths that offer it — the row's close button and `terminal.close-window`.
An agent mid-run, a build, an editor holding unsaved work: one press, no
warning, nothing to undo.

This app already classifies terminal activity: `SessionsService.SessionStatuses`
reports ready/active/approval per window, and it gets there by capturing the
pane and matching what an agent printed. That answers a different question from
a different source. It is about agents, so a build, a test run or an editor is
invisible to it, and it reads output, so what it knows is whatever the program
chose to draw. Guarding a close with it would cover the one case that already
has an indicator on the row and miss every other case worth guarding.

## Decision

`TerminalsService.WindowForeground` answers what a window is running from
process state, `POST /api/terminal/windows/foreground` carries it, and every
user-initiated close asks it first. `running` is false only when every live pane
in the window is a shell waiting at its prompt; anything else confirms, and the
answer names the process so the dialog can say what it is about to stop.

Two checks per pane, and neither is enough alone:

- **The name.** `#{pane_current_command}` is the pane's foreground process,
  which is what tells `claude` from `zsh`. On its own it reads a running
  `#!/bin/bash` script as a prompt, because a script runs under its
  interpreter's own name.
- **The process.** Whether `#{pane_pid}` holds the foreground process group of
  its own tty — gopsutil's `Foreground`, which is the `+` in `ps -o stat=` on
  macOS and `pgrp == tpgid` in `/proc` on Linux. On its own it reads an agent as
  a prompt, because tmux runs a window's command through `sh -c`, which execs it
  in place: the pane's own process *becomes* the work rather than parenting it.

Uncertainty answers `running`. A pid that cannot be read, a window whose panes
cannot be listed, a request that fails — each confirms rather than closes. The
two ways of being wrong do not cost the same: a dialog nobody needed against a
process killed without one.

The question is asked at the moment of the close and not carried on the window
listing the sidebar already sweeps. What a pane is running changes without tmux
announcing anything, so a swept answer would be a stale one, and the sweep would
pay for a probe per window it mostly does not need.

The answer covers the whole window rather than its active pane, because closing
a window kills every pane in it; the active pane's process is the one named when
several are running, since it is the one on screen.

## Consequences

An idle shell tab still closes on the click. The cost of the guard is one
round trip on the loopback control plane — one tmux command, plus a process read
per pane, which on macOS is a `ps` — and it is paid only where a close was
asked for.

`shells` in `terminals_service.go` is the one list a new interactive shell has to
be added to. A shell that is not on it reads as work and confirms, which is the
safe direction for a list that will always be incomplete.

Backgrounded and stopped jobs are not covered: a shell with `sleep 300 &` behind
it is still the foreground process, so the tab closes without asking. The check
is named for what it reads, and widening it to any child process would confirm
on every shell that has ever suspended something.

The desktop now reads OS process state for a terminal decision, where it
previously only ever asked tmux. It is one call through `gopsutil`, already a
dependency for `procstats`, behind a field on the service so a test can drive
the answer without arranging the process states it stands for.
