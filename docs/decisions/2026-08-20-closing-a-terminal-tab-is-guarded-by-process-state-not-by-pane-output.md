# Closing a terminal tab is guarded by process state, not by pane output

- **Status:** accepted
- **Date:** 2026-08-20

## Context

Closing a tab kills its tmux window and everything in it. A close needs a guard
for an agent mid-run, a build, or an editor with unsaved work. The app's agent
status (`SessionStatuses`) reads pane output and only knows about agents, so it
would miss a build, a test run, or an editor.

## Decision

`TerminalsService.WindowForeground` answers from process state whether closing
a window would kill work, served at `POST /api/terminal/windows/foreground`
(and `PaneForeground` at `/api/terminal/panes/foreground` for one pane). Every
user-initiated close asks first. `running` is false only when every live pane
is a shell waiting at its prompt; anything else confirms, and the answer names
the process so the dialog can say what it will stop.

A pane is at a prompt only when both checks pass:

- **The name.** `#{pane_current_command}` is on the `shells` list. Alone, it
  reads a running `#!/bin/bash` script as a prompt.
- **The process.** `#{pane_pid}` holds the foreground process group of its tty
  (gopsutil's `Foreground`). Alone, it reads an agent as a prompt, because tmux
  runs a window's command through `sh -c`, which execs it in place.

Uncertainty answers `running`: an unreadable pid, an unlistable window, or a
failed request confirms rather than closes. The question is asked at close
time, not swept with the window listing, because what a pane runs changes
without tmux announcing it. The answer covers the whole window, naming the
active pane's process when several run.

## Consequences

- `shells` in `terminals_service.go` is the one list a new interactive shell
  must be added to. A shell not on it reads as work and confirms, the safe
  direction.
- Backgrounded and stopped jobs are not covered: a shell with `sleep 300 &`
  behind it is still in the foreground, so the tab closes without asking.
