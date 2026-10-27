# Discover the tmux binary instead of trusting $PATH

- **Status:** accepted
- **Date:** 2026-07-29

## Context

The desktop execs tmux for terminal mode's control client (ADR
terminal-transport) and for hive session lifecycle through the shared tmux
adapter. A desktop launch does not inherit the user's shell `$PATH`: macOS
starts an `.app` bundle with `/usr/bin:/bin:/usr/sbin:/sbin`, so
`/opt/homebrew/bin/tmux` and other package-manager installs are invisible to
the app while every terminal finds them.

## Decision

1. **`internal/platform/tmux/bin` (package `tmuxbin`) owns static
   discovery.** The order is the `paths.tmux` setting, then `$PATH`, then the
   prefixes package managers install into (`execenv.SearchDirs`). When none
   holds tmux, `App` asks `execenv` for `tmux` on the login-shell PATH. A
   configured path is used or fails, never fallen back from: silently running
   a different tmux than the configured one is worse than an error.

2. **A successful lookup is remembered; a failure is not**
   (`tmuxbin.Resolver`), so installing tmux takes effect without a relaunch.
   Changing `paths.tmux` does need one, because it is read at composition.

3. **The tmux clients hold no discovery policy.** `control.ManagerOptions`
   takes `Binary func() (string, error)`, which the composition root supplies
   and the manager calls on every failed probe. Hive session lifecycle gets
   the same resolver through the tmux exec runner, with Desktop's resolved
   subprocess environment and the inherited tmux socket.

## Consequences

- A stock Homebrew or Nix install works from the Dock with no configuration.
- A custom install found only on the shell PATH pays `execenv`'s bounded
  login-shell probe. `paths.tmux` is the strict escape hatch.
- `paths.tmux` must be absolute (validated). Whether the binary exists is not
  validated: a missing tmux shows as terminal-unavailable with a reason that
  names the setting, not as a settings file the app refuses to load.
