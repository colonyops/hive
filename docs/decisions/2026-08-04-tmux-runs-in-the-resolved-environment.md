# tmux is spawned in the resolved environment, because a login shell is not an interactive one

- **Status:** accepted
- **Date:** 2026-08-04

## Context

The app resolves one environment for the commands it spawns on the user's
behalf (ADR subprocess-environment). tmux looked like it did not need it,
because panes start login shells. An agent session is created with
`new-session ... -- $SHELL -l -c <launch line>`, though, and a login shell
running `-c` is not interactive: zsh never reads `.zshrc`, where most PATH
additions live. Launched from the Dock (launchd's minimal PATH) or relaunched by
Homebrew, the tmux client passed that PATH to the pane, the agent binary was not
found, and the session exited at once.

## Decision

1. **`tmuxcc.Manager` takes an `Environ` hook** (`ManagerOptions.Environ`), the
   same shape as `ptyterm`'s and wired to the same `execenv.Resolver` at the
   composition root. The two backends stay siblings with no shared interface
   (ADR ephemeral-popup-terminals).
2. **It reaches every tmux the manager execs**, the control client and the
   one-shots, because a pane inherits the environment of the client that
   created it. Setting `Cmd.Env` on the `new-session` client is enough; no
   `new-session -e` plumbing.
3. **The resolved environment is a floor, not a replacement.** `-l` still runs,
   so a login startup file may still set a PATH that wins.
4. **The Desktop's shared tmux runner gets the same hook**
   (`newTmuxRunner`, through `tmuxcc.RunnerEnviron`), so session lifecycle and
   pane-capture status use the same environment as control-mode attaches.
5. **A session that dies before it can be attached says so on screen.** The
   launch response's notice is the only record of why, so the Agents pane shows
   it instead of returning to idle.

## Consequences

- Everything the app spawns on the user's behalf now goes through `execenv`,
  including tmux. `Environ` carries the whole shell environment, not just PATH
  (ADR a-subprocess-inherits-the-whole-shell-environment-not-just-its-path).
- The first tmux operation of a run may wait on the login-shell probe (bounded
  at five seconds, run once per process). A failed probe degrades to the
  inherited environment.
- A tmux session created before the app touched the server keeps the
  environment it was created with. Fixing it means killing and restarting it.
