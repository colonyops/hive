# Shutdown is signalled, bounded, and owned above the dev runner

- **Status:** accepted
- **Date:** 2026-07-30

## Context

`desktop/main.go` ran its teardown after `ui.Run()` returned, and nothing else
reached it.

Four failures followed, all of them found by reproducing a quit rather than by
reading the code:

1. **On macOS the teardown never ran at all.** `Quit` there is
   `[NSApp terminate:]`, which runs the application's shutdown hooks from
   `applicationShouldTerminate:` and exits the process — `Run` does not return,
   so the code after it is unreachable. Every quit was affected, the tray's own
   Quit item included; the `server` build was the only one where returning from
   `Run` ever happened, which is why `serve` and e2e never showed it.

2. **The app had no signal disposition.** SIGINT, SIGTERM and SIGHUP killed it
   on Go's default action. After a SIGTERM the pipeline database was left with
   a 4.2 MB `-wal` and a live `-shm`: nothing had closed it.

3. **`App.Close`'s shutdown budget was not enforced.** It gives the terminal
   manager three seconds, but `Manager.Stop` closed clients one at a time and
   `Client.teardown` joins the reader, the command worker and the tmux child
   with no deadline of its own — and the detach it sends first writes to a pipe
   nothing guarantees is drained. A single client that could not come down held
   the whole quit open forever, which a test now demonstrates against the old
   code by timing out.

4. **The dev runner leaked a running app.** `wails3 dev` starts the app and the
   Vite task with `Setpgid`, because killing a process group is how its file
   watcher restarts them. A closing terminal signals only the foreground
   group — the task shell and `wails3 dev` — and neither the runner nor its
   watcher library registers SIGHUP, so the runner dies on the default action
   before tearing anything down. Reproduced with `tmux kill-session`: the
   runner and mise were gone, the app and Vite survived with `ppid 1`. The same
   library also only ever SIGKILLs the app, on Ctrl+C and on every hot reload,
   so no signal handler in the app can help inside the dev loop.

## Decision

1. **One teardown, reached from every exit.** It is registered as a Wails
   shutdown hook *and* called after `Run` returns, guarded by a `sync.OnceFunc`
   so the pair is exactly one teardown. Which of the two fires is the
   platform's business, and the hook is the half that reaches macOS at all.
   SIGINT, SIGTERM and SIGHUP are answered with `UI.Quit`, so a signal is just
   another way to ask for the quit the tray already asks for. `os/signal`
   belongs to the process, not the core, so it is wired at the composition
   root — `internal/app` still registers none (see `App.Close`, and the
   goroutine count `TestAppLifecycle` asserts).

2. **A signalled shutdown is bounded.** A watchdog exits the process after
   `shutdownGrace`, or immediately on a second signal. Quit hands work to the
   main thread and the teardown joins terminal clients and drains an HTTP
   server; any of that can wedge, and an app that cannot be killed is worse
   than one that skipped its teardown.

3. **`Manager.Stop` closes clients concurrently and joins under the caller's
   context.** What the deadline abandons is the join, not the teardown: every
   client has already been sent its kill by then. This is what makes the three
   seconds `App.Close` documents a real bound.

4. **The dev session is supervised above the runner.** `mise run dev`
   launches `cmd/devtools run`, which starts `wails3 dev` in a process group of
   its own so terminal signals arrive at the supervisor and nowhere below it.
   On a signal it asks the app to shut down, waits for it, interrupts the
   runner so the runner's own teardown clears Vite, and finally sweeps whatever
   from the pre-teardown process tree is still alive.

5. **The supervisor does not trust the signal to arrive.** It also watches the
   session leader — the shell that owns the terminal — and tears down when that
   goes away. A pane killed out from under a process group that is not the
   terminal's foreground one delivers nothing at all, which is why the leaked
   sessions found on this machine were still running with revoked stdio. Every
   way a terminal can end therefore converges on the same teardown, and this is
   the half that does not depend on `solo down` or a shell forwarding anything.

## Consequences

- Every quit now runs `App.Close`: the databases close, the tmux control clients
  detach, and the loopback server drains. Verified by the absence of `-wal` and
  `-shm` beside `desktop-pipeline.db` after a Ctrl+C, where before both were
  left behind.
- Closing the terminal — a window, a tmux pane, `solo down` — no longer leaves
  the app running.
- The supervisor identifies the app by argv[0] base name (`hive-desktop`),
  scoped to descendants of its own runner. The scoping is the load-bearing
  part: several worktrees run dev sessions at once and a sweep must never reach
  into another one. It re-reads the process table and compares the command
  before killing, so a recycled pid is not killed for its predecessor.
- The supervisor owns only teardown. Rebuild-and-restart stays the runner's, so
  a hot reload still SIGKILLs the app the way it always did.
- `wails3 dev` still does not exit when the app exits on its own — quitting from
  the tray leaves the dev session supervising nothing until it is interrupted.
  That is upstream behaviour and is left alone; it no longer leaks, because the
  session's own teardown now works.
- The dev path gains a `go run` of `cmd/devtools`. It is already how `prepare`,
  `fresh` and `reset` run.
