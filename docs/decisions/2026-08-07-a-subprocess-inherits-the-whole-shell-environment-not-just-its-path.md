# A subprocess inherits the whole shell environment, not just its PATH

- **Status:** accepted; supersedes the deferral in
  [ADR subprocess-environment](2026-07-30-subprocess-environment.md) and the
  `Environ` consequence of
  [ADR hive-env-overrides-resolve-through-the-login-shell](2026-08-06-hive-env-overrides-resolve-through-the-login-shell.md)
- **Date:** 2026-08-07

## Context

ADR subprocess-environment adopted the login shell's PATH and deferred the rest:
"widening to more variables is a contained change behind `Environ`, deferred
until a hook demonstrably needs a non-PATH variable."

`EDITOR` is the demonstration (#279). It is set in `.zshrc`, the probe reads it,
and it never reaches a child: an agent CLI's "open in editor" binding had nothing
to resolve and fell back to whatever it could find on PATH. Verified against a
live session with `ps eww` — PATH fully resolved to 40 entries, `EDITOR` and
`VISUAL` absent. Not a probe failure; the probe already had the value.

ADR hive-env-overrides-resolve-through-the-login-shell made the probe keep its whole answer, and gave one reason
for still not merging it: adopting the shell's variables wholesale "would let a
startup file shadow what the app deliberately sets for a child." That is an
argument about precedence, not about which variables exist.

## Decision

1. **Everything the probe reported is adopted, minus a closed list.**
   `shellSessionVars` — `SHLVL`, `_`, `PWD`, `OLDPWD`, `TERM`, `TMUX`,
   `TMUX_PANE`, `SHELL` — describe the probe shell's own process and are wrong
   for any child by construction; `TMUX` would tell a spawned process it is
   inside a tmux client that it is not. An allowlist was the alternative and is
   the open set this ADR's predecessor already refused once: enumerating PATH
   prefixes could not cover an open set of hook commands, and enumerating
   variable names cannot cover an open set of tools that read them. A denylist
   of things that are wrong is closed; a list of things that are wanted is not.

2. **This process wins every name it defines; the shell fills the rest.**
   The same rule `Getenv` already applies, for the same reason — a launch that
   names a variable is more specific than a startup file. It is what keeps a
   stale rc file from shadowing a `HIVE_DESKTOP_*` override, which was the
   objection that deferred this. PATH stays the one merge rather than a
   substitution, unchanged from ADR subprocess-environment.

   Defined means present, not non-empty. `overrides.env` opts out of a default
   by setting it to nothing (`HIVE_DESKTOP_DEVELOPMENT_GITHUB_API_BASE=""`), and
   a startup file must not refill it. `Getenv` differs deliberately: it answers
   what value a terminal would show, where empty and unset are one answer.

3. **A variable is adopted only under a name that is one.** bash exports
   functions as `BASH_FUNC_x%%=() {…}`, a value spanning lines `env` gives no way
   to delimit, so the parse takes the first line and drops the rest. Inert while
   only PATH was read; now it would hand every child a definition its shell fails
   to parse. A line is an assignment only when nothing before its first `=` is
   whitespace, its name must be `[A-Za-z_][A-Za-z0-9_]*`, and any line that is
   neither continues the value above it — so a multi-line value arrives whole and
   a rejected one takes its own continuation lines with it rather than appending
   them to the variable printed before it. Continuations are not hypothetical:
   mise exports a multi-line `__MISE_ZSH_ACTIVATE_ENV`.

4. **`.zshrc` being interactive-only is not a reason to withhold it.** A spawn is
   not an interactive shell by the shell's definition, but what these sessions
   hold — agent CLIs and TUI editors — is interactive in every other sense. The
   probe has been `-i` since ADR subprocess-environment for the same reason.

## Consequences

- Every consumer of `Environ` widens at once, which is what the seam was for:
  `app.envExecutor`, `dispatch.ShellExecutor`, `sources.exec`, the editor launch
  in `AgentWorkspacesService`, and both terminal backends
  (ADR tmux-runs-in-the-resolved-environment, ADR ephemeral-popup-terminals). No consumer changed.
- **The whole shell comes with `EDITOR`, secrets included.** Simulating a Dock
  launch against a real `.zshrc` adopted 57 variables, among them `SSH_AUTH_SOCK`
  and `LANG` — wanted — but also `BW_SESSION`, `SOPS_AGE_KEY_FILE` and
  `MISE_GITHUB_CREDENTIAL_COMMAND`. This is accepted, not overlooked: a terminal
  hands those to every command the user runs, and "behaves like the user's
  terminal" is the whole requirement. It does mean a `sources.exec` command or an
  agent session sees credentials it could not see before, so a variable a user
  does not want spawned commands to hold belongs in neither `.zshrc` nor
  `.zshenv`.
- A shell's own bookkeeping is adopted too — `__MISE_SESSION`, `__MISE_DIFF`,
  `STARSHIP_SESSION_KEY`. Left in deliberately: they are what a terminal's child
  gets, a launch from a terminal has always passed them through `os.Environ()`,
  and denying them by prefix would be the allowlist-chasing decision 1 rejected.
  A concrete breakage makes one more entry in `shellSessionVars`, not a rethink.
- Both terminal backends keep scrubbing the session variables they already
  scrubbed (`ptyterm.terminalEnv`, `tmuxcc.detachedEnv`). The denylist stops the
  *shell's* values; theirs stop this process's, which a launch from inside tmux
  still supplies, and `detachedEnv` also serves a caller that passes no
  environment at all.
- The environment is fixed at the probe, so a startup file edited mid-run
  reaches a child on the next launch — the same restart ADR hive-env-overrides-resolve-through-the-login-shell already
  documented for `Getenv`.
- Nothing new is spawned. The probe was already running, already interactive,
  and already keeping its whole answer; this ADR only stops discarding it.
