# Discover the tmux binary instead of trusting $PATH

- **Status:** accepted
- **Date:** 2026-07-29

## Context

Two things in the app exec tmux: terminal mode's control client
(`internal/app/tmuxcc`, ADR terminal-transport) and Hive session spawning, which runs `tmux`
through the vendored core's `executil.Executor`. Both used the bare name, so
both depended on `$PATH`.

A desktop launch does not inherit the user's shell `$PATH`. macOS starts an
`.app` bundle with `/usr/bin:/bin:/usr/sbin:/sbin`, which contains no Homebrew,
MacPorts or Nix prefix — so `/opt/homebrew/bin/tmux`, the standard install on
Apple silicon, is invisible to the app while every terminal on the machine finds
it. The symptom is terminal mode reporting tmux missing, and session spawn
failing, on a machine that has tmux.

## Decision

1. **`internal/app/tmuxbin` is the one place tmux is located.** Order is the
   `paths.tmux` setting, then `$PATH`, then the prefixes package
   managers install into, in the precedence a shell would have applied
   (Homebrew before `/usr/bin`). A configured path is used or fails — never
   fallen back from, because silently running a different tmux than the one
   configured is worse than an error the user can act on.

2. **A successful lookup is remembered; a failure is not.** Installing tmux
   takes effect without relaunching the app — the policy `tmuxcc.Manager`
   already had for its version probe. Changing `paths.tmux` itself does need a
   relaunch: the override is read from the settings snapshot at composition
   time, like every other startup-read setting.

3. **`tmuxcc` holds no discovery policy.** `ManagerOptions.Binary` is a
   `func() (string, error)` the composition root supplies and the manager calls
   on every failed probe; `Options.Binary` carries the resolved path to the
   attach. Left unset it is `tmux`, so tests and the server build are unchanged.

4. **Session spawn gets the same binary through a decorated executor.**
   `app.tmuxExecutor` wraps `executil.Executor` and substitutes the located path
   for the command name `tmux`. The vendored tree stays untouched (VENDOR.md
   forbids editing it, and this is not a change to make upstream: the PATH a GUI
   inherits is not hive's problem). When discovery fails the decorator passes
   `tmux` through, so the error the user sees is the caller's own.

## Consequences

- A stock Homebrew or Nix install works from the Dock with no configuration,
  which is the point.
- The search list is a heuristic that will drift; adding a prefix is a one-line
  change, and `paths.tmux` is the escape hatch in the meantime. It is
  deliberately not "run the user's shell and read its PATH" — that executes a
  login shell at startup to learn one path, and inherits whatever that shell
  does.
- `paths.tmux` must be absolute (validated), because a relative value
  would be resolved against `$PATH` or the working directory, which is what
  setting it opts out of. Whether the binary exists is not validated: a missing
  tmux surfaces as terminal-unavailable, not as a settings file the app refuses
  to load.
- The terminal's unavailable reason now names the setting, and a successful
  probe logs which tmux the run resolved — the first thing worth knowing from a
  problem report (ADR in-app-problem-reporting).
