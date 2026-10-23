# Run the user's commands in the user's PATH

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Session creation runs the `commands` of matched rules as `sh -c <command>` in
the new session's directory, and a failure aborts the creation. Shell actions
(`actions.yml`) are the same shape. Both are the user's own commands, written
against their terminal's PATH.

A desktop launch's environment is the launcher's: macOS starts an `.app` bundle
with `/usr/bin:/bin:/usr/sbin:/sbin`. A hook calling `mise`, `pnpm` or `just`
fails with exit status 127 where the same line works in every terminal (#155).
ADR tmux-discovery covers the one binary the app itself execs; hooks are an
open set, so no list of prefixes covers them.

## Decision

1. **`internal/platform/execenv` resolves one PATH for every command the app
   spawns on the user's behalf.** It is the user's login shell's PATH, then this
   process's, then the package-manager prefixes from `SearchDirs`, first
   occurrence winning. The rest of the shell's environment is adopted too (ADR
   a-subprocess-inherits-the-whole-shell-environment-not-just-its-path).

2. **The login shell is asked once per run, and the answer is kept whether or
   not it worked.** `$SHELL -ilc /usr/bin/env`, five-second timeout.
   Interactive as well as login, because PATH is as often set in `.zshrc` as in
   a login file. A program is run rather than a variable echoed, because fish
   joins a list variable with spaces. The probe runs on a context detached from
   the caller's, so a cancelled session creation is not recorded as the shell's
   answer. On Unix it runs with `Setsid`, so an interactive shell's job control
   cannot claim the app's foreground process group and strand Ctrl+C.

3. **A failed probe degrades, it does not fail.** The inherited PATH plus the
   prefixes is the fallback.

4. **The desktop's executor (`newEnvExecutor`) hands every child the resolved
   environment and resolves the command name through `execenv.LookPath`.**
   `os/exec` searches the calling process's PATH and ignores `Cmd.Env`, so a
   binary present only in the resolved PATH would otherwise never start. A name
   that cannot be resolved is passed through, so the caller sees the OS's own
   error.

5. **A failing streamed command carries the first 2 KiB of its stderr in its
   error.** Hive streams hook output to `io.Discard` in the desktop, so without
   this a failed hook shows only an exit status.

## Consequences

- Version-manager shims (mise, asdf) work from the Dock, because they are on
  the shell's PATH.
- The app runs one login shell per run, lazily on the first command it spawns,
  and inherits whatever that shell's startup costs, once.
- tmux discovery does not consult the probe. tmux itself runs in the resolved
  environment (ADR tmux-runs-in-the-resolved-environment).
- The resolved PATH is logged (`info` for the source, `debug` for the value),
  so a problem report (ADR problem-reports-are-github-issues) shows which environment a
  failing hook had.
