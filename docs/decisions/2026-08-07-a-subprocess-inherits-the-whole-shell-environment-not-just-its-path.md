# A subprocess inherits the whole shell environment, not just its PATH

- **Status:** accepted
- **Date:** 2026-08-07

## Context

A launched `.app` does not see what the user's startup files export. ADR
subprocess-environment adopted the login shell's PATH, and the probe already
read every variable (ADR hive-env-overrides-resolve-through-the-login-shell),
but only PATH reached a child. `EDITOR` set in `.zshrc` never reached an agent
CLI, so its "open in editor" binding had nothing to resolve.

## Decision

`execenv.Resolver.Environ` is the environment every subprocess runs with.

1. **Everything the probe reported is adopted, minus a closed list.**
   `shellSessionVars` (`SHLVL`, `_`, `PWD`, `OLDPWD`, `TERM`, `TMUX`,
   `TMUX_PANE`, `SHELL`) describe the probe shell's own process and are wrong
   for any child. A denylist of what is wrong is closed; an allowlist of what
   is wanted cannot cover an open set of tools.
2. **This process wins every name it defines; the shell fills the rest.** A
   launch that names a variable is more specific than a startup file, which
   keeps a stale rc file from shadowing a `HIVE_DESKTOP_*` override. PATH is
   the one merge rather than a substitution. Defined means present, not
   non-empty: `overrides.env` opts out of a default by setting it to nothing,
   and a startup file must not refill it. `Getenv` differs on purpose, since it
   answers what a terminal would show, where empty and unset are the same.
3. **A variable is adopted only under a valid name.** A line is an assignment
   only when nothing before its first `=` is whitespace and the name matches
   `[A-Za-z_][A-Za-z0-9_]*`. Any other line continues the value above it, so a
   multi-line value (mise's `__MISE_ZSH_ACTIVATE_ENV`) arrives whole and a
   rejected one (bash's `BASH_FUNC_x%%`) takes its continuation lines with it.
4. **The probe is interactive (`-ilc`)**, because what sessions run (agent
   CLIs, TUI editors) is interactive in every sense that matters and PATH is as
   often set in `.zshrc` as in a login file.

## Consequences

- **The whole shell comes along, secrets included.** A spawned command or agent
  session sees every credential the user's `.zshrc` exports, as a terminal
  would. A variable a user does not want spawned commands to hold belongs in
  neither `.zshrc` nor `.zshenv`.
- Shell bookkeeping (`__MISE_SESSION`, `STARSHIP_SESSION_KEY`) is adopted too.
  A concrete breakage adds one entry to `shellSessionVars`, not a prefix rule.
- The terminal backends still scrub their own variables
  (`ptyterm.terminalEnv`, `withoutTmuxClient` in the tmux control package).
  The denylist stops the shell's values; theirs stop this process's, which a
  launch from inside tmux still supplies.
- The environment is fixed at the probe, so an edited startup file reaches a
  child on the next launch.
