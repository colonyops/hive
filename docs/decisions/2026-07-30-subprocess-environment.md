# Run the user's commands in the user's PATH

- **Status:** accepted; the last consequence below is superseded by [ADR tmux-runs-in-the-resolved-environment](2026-08-04-tmux-runs-in-the-resolved-environment.md)
- **Date:** 2026-07-30

## Context

Session creation runs the hooks configured in `hive.yaml`'s rules: a matched
rule's `commands` are executed as `sh -c <command>` in the new session's
directory, and a failure aborts the creation. Shell actions (`actions.yml`) are
the same shape.

Both are the user's own commands, written against the PATH their terminal has.
Neither got it: `executil.RealExecutor` leaves `Cmd.Env` nil, so a child
inherits the app's environment, and a desktop launch's environment is the
launcher's — macOS starts an `.app` bundle with
`/usr/bin:/bin:/usr/sbin:/sbin`. A hook calling `mise`, `pnpm`, `just` or
anything else a package or version manager installed therefore fails with exit
status 127 on a machine where the same line works in every terminal, and the
session cannot be created at all (#155). ADR tmux-discovery fixed this for the one binary
the app itself execs; hooks are an open set, so no list of prefixes covers them.

It failed silently as well: hive streams hook output to the writers the desktop
supplies, and the desktop supplies `io.Discard`, so the shell's own
"command not found" was dropped and the jobs list showed a bare exit status.

## Decision

1. **`internal/app/execenv` resolves one PATH for every command the app spawns
   on the user's behalf.** It is the user's login shell's PATH, then this
   process's, then the package-manager prefixes from ADR tmux-discovery — first
   occurrence winning, so a directory both name is searched once.

2. **The login shell is asked, once per run, and the answer is kept whether or
   not it worked.** `$SHELL -ilc /usr/bin/env`, five-second timeout, PATH read
   out of the child's environment. Interactive as well as login, because the
   PATH a terminal shows is as often set in `.zshrc` as in a login file. A
   program is run rather than a variable echoed because the syntax to echo one
   is not portable — fish joins a list variable with spaces, which would produce
   something that is not a PATH at all. A failure is remembered too: a shell's
   PATH does not change under a running app, and re-probing would charge every
   later hook command that shell's startup cost. The probe runs on a context
   that outlives the caller's, so cancelling a session creation mid-probe is not
   recorded as the shell's answer. On Unix it also runs in a separate session:
   an interactive shell enables job control, and sharing the app's controlling
   terminal lets the probe claim its foreground process group. The probe then
   exits and leaves Ctrl+C aimed at a dead group instead of the app or its dev
   supervisor.

3. **A failed probe degrades, it does not fail.** The inherited PATH plus the
   prefixes is what the app could reach before this ADR, so the worst case is
   the old behaviour rather than a session that cannot be created because a
   shell would not answer.

4. **`app.envExecutor` replaces `executil.RealExecutor`.** The vendored
   implementation exposes no seam for a child's environment and
   `internal/hivecore` stays untouched, so the app owns the four-method
   implementation and hands `Cmd.Env` the resolved environment. It resolves the
   command name itself through `execenv.LookPath`: `os/exec` searches the
   *calling* process's PATH and ignores `Cmd.Env`, so a binary present only in
   the resolved PATH would otherwise never start. A name that cannot be resolved
   is passed through, so the caller sees the OS's own error for what it asked
   for.

5. **A failing streamed command carries the opening of its stderr in its
   error.** Bounded at 2 KiB. This is what turns "exit status 127" in the jobs
   list into the shell naming the command it could not find.

## Consequences

- A hook that works in the user's terminal works from the Dock, which is the
  point. Version-manager shims (mise, asdf) come with it — they are on the
  shell's PATH and could never have been enumerated.
- The app runs one login shell per run, lazily on the first command it spawns,
  and inherits whatever that shell does. That is the cost ADR tmux-discovery declined to
  pay to locate a single binary; for an open set of user commands there is no
  list that substitutes for it. tmux discovery is unchanged and still does not
  consult the probe — though when discovery finds nothing, the bare name falls
  through to `envExecutor`, whose PATH includes the probe's answer: a
  deliberate last-resort rescue rather than a hard failure.
- A hook that is slow because the user's shell is slow to start pays that once,
  not per command.
- ~~The probe captures the whole environment but only PATH is adopted. Widening
  to more variables (the full-environment model VS Code and JetBrains use) is
  a contained change behind `Environ`, deferred until a hook demonstrably
  needs a non-PATH variable.~~ **Superseded by
  [ADR a-subprocess-inherits-the-whole-shell-environment-not-just-its-path](2026-08-07-a-subprocess-inherits-the-whole-shell-environment-not-just-its-path.md).**
  `EDITOR` was the demonstration, and the change was as contained as predicted.
- The PATH a run resolved is logged (`info` for the source, `debug` for the
  value), so a problem report (ADR in-app-problem-reporting) says which environment a failing hook
  actually had.
- ~~Anything the app spawns that does not go through `envExecutor` or
  `dispatch.ShellExecutor` still gets the inherited environment —
  `tmuxcc`'s control client is the live example, and it does not need the PATH
  because ADR tmux-discovery hands it an absolute binary and tmux's own panes start login
  shells.~~ **Superseded by ADR tmux-runs-in-the-resolved-environment.** A pane's login shell is not an
  interactive one, so it never reads the file the PATH is usually set in;
  `tmuxcc` takes the resolved environment too.
