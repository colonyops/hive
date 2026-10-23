# Shutdown is signalled, bounded, and owned above the dev runner

- **Status:** accepted
- **Date:** 2026-07-30

## Context

Teardown code placed after `ui.Run()` does not run on macOS: `Quit` there is
`[NSApp terminate:]`, which runs the shutdown hooks and exits without `Run`
returning. With no signal disposition, SIGINT, SIGTERM and SIGHUP kill the app
on Go's default action and leave the databases open. A tmux client that cannot
come down can hold a quit open forever.

The dev runner has its own gap. `wails3 dev` starts the app and Vite in their
own process groups, neither it nor its watcher library registers SIGHUP, and
it only ever SIGKILLs the app. Closing the terminal kills the runner and
leaves the app and Vite running with `ppid 1`.

## Decision

1. **One teardown, reached from every exit.** `cmd/desktop/main.go` registers
   it as a Wails shutdown hook and also calls it after `Run` returns, guarded
   by `sync.OnceFunc`. SIGINT, SIGTERM and SIGHUP are answered with `UI.Quit`.
   `os/signal` is wired at the composition root, never in `internal/app`.

2. **A signalled shutdown is bounded.** A watchdog exits the process after
   `shutdownGrace`, or at once on a second signal. An app that cannot be killed
   is worse than one that skipped its teardown.

3. **The tmux `Manager.Stop` closes clients concurrently and joins under the
   caller's context.** The deadline abandons the join, not the teardown, so the
   budget `App.Close` gives it is a real bound.

4. **The dev session is supervised above the runner.** `mise run desktop:dev`
   launches `devtools run` (`cmd/desktop/devtools/devsession.go`), which starts
   `wails3 dev` in a process group of its own so terminal signals reach the
   supervisor. On a signal it asks the app to shut down, waits, interrupts the
   runner so it clears Vite, then sweeps whatever from the pre-teardown process
   tree is still alive.

5. **The supervisor also watches the session leader** and tears down when it
   goes away. A pane killed under a process group that is not the terminal's
   foreground one delivers no signal at all.

## Consequences

- The sweep identifies the app by argv[0] base name (`hive-desktop`), scoped to
  descendants of its own runner. The scoping matters: several worktrees run dev
  sessions at once, and a sweep must never reach into another. It re-reads the
  process table before killing, so a recycled pid is safe.
- The supervisor owns only teardown. A hot reload still SIGKILLs the app.
- `wails3 dev` does not exit when the app quits on its own; that is upstream
  behaviour and is left alone.
