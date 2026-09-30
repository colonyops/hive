package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
	"golang.org/x/sys/unix"
)

const (
	// appProcessName is the dev app's argv[0] base name. The darwin run task
	// launches it from inside a .app bundle and the linux one straight out of
	// bin/, but both keep APP_NAME, so the base name identifies it on either.
	appProcessName = "hive-desktop"

	// appGrace outlasts the app's own shutdown watchdog (desktop/main.go), so
	// reaching it means the app is not going to exit on its own and the runner's
	// kill is what will end it.
	appGrace = 12 * time.Second

	// runnerGrace is how long the dev runner gets, after its interrupt, to kill
	// the process groups it created and exit.
	runnerGrace = 10 * time.Second

	// aliveInterval is how often a wait polls. Signalled processes are not our
	// children — they belong to the runner — so there is nothing to wait(2) on.
	aliveInterval = 50 * time.Millisecond

	// terminalInterval is how often the owning terminal is checked. It bounds
	// how long a dev session outlives the window it was started in.
	terminalInterval = 2 * time.Second
)

// runDevSession runs the Wails dev runner as a supervised child and owns the
// teardown the runner does not do itself. Two gaps make it necessary, both only
// visible on the way out:
//
//   - The runner puts the app and the Vite task in process groups of their own,
//     because killing a group is how its file watcher restarts them. A terminal
//     that closes signals only the foreground group — the task shell and the
//     runner — and neither the runner nor the watcher library registers SIGHUP,
//     so the runner dies on Go's default disposition before it can tear anything
//     down. The app is left running with no dev session behind it. That leak is
//     what this exists to close.
//   - The runner only ever SIGKILLs the app. Asking it to close first is what
//     gives the dev loop the same graceful shutdown a packaged build gets.
//
// The runner is started in its own process group so terminal signals arrive
// here and nowhere below: the ordering — app first, runner second — is the
// whole point, and it is lost if the runner starts killing groups on its own
// copy of the same Ctrl+C. A close that delivers no signal at all is caught by
// terminalClosed instead, so neither half of this depends on the other.
func runDevSession(logger zerolog.Logger, argv []string) error {
	runner := exec.Command(argv[0], argv[1:]...) //nolint:noctx // lifetime is this supervisor's, not a request's
	runner.Stdin, runner.Stdout, runner.Stderr = os.Stdin, os.Stdout, os.Stderr
	runner.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)

	if err := runner.Start(); err != nil {
		return fmt.Errorf("start dev runner %q: %w", argv[0], err)
	}
	logger.Debug().Int("pid", runner.Process.Pid).Str("runner", strings.Join(argv, " ")).Msg("dev session started")

	exited := make(chan error, 1)
	go func() { exited <- runner.Wait() }()

	select {
	case err := <-exited:
		return runnerExit(err)
	case sig := <-signals:
		logger.Info().Str("signal", sig.String()).Msg("stopping the dev session")
	case <-terminalClosed(logger):
		logger.Info().Msg("terminal closed; stopping the dev session")
	}

	// A second signal is the user saying they are done waiting, so every grace
	// period below collapses at once rather than each in turn.
	urgent := make(chan struct{})
	go func() {
		sig := <-signals
		logger.Warn().Str("signal", sig.String()).Msg("second signal; not waiting for a clean shutdown")
		close(urgent)
	}()

	// Snapshotted before anything is signalled: once the runner exits, its
	// children are reparented and the tree that identifies them is gone.
	tree := descendantsOf(logger, runner.Process.Pid)
	stopApp(logger, tree, urgent)

	// The runner's own interrupt path kills the process groups it created, which
	// is what clears Vite and whatever the app left behind.
	if err := runner.Process.Signal(os.Interrupt); err != nil {
		logger.Debug().Err(err).Msg("interrupting the dev runner")
	}

	var runErr error
	select {
	case runErr = <-exited:
	case <-urgent:
		runErr = killRunner(logger, runner, exited)
	case <-time.After(runnerGrace):
		logger.Warn().Dur("grace", runnerGrace).Msg("dev runner did not stop in time; killing it")
		runErr = killRunner(logger, runner, exited)
	}

	sweep(logger, tree)
	return runnerExit(runErr)
}

// killRunner force-kills the runner's process group. Addressing it by pid is
// safe where the sweep's is not: the runner is this process's child and has not
// been reaped, so the number cannot yet have been recycled.
func killRunner(logger zerolog.Logger, runner *exec.Cmd, exited <-chan error) error {
	if err := syscall.Kill(-runner.Process.Pid, syscall.SIGKILL); err != nil {
		logger.Debug().Err(err).Msg("killing the dev runner")
	}
	select {
	case err := <-exited:
		return err
	case <-time.After(time.Second):
		return nil
	}
}

// terminalClosed fires when the session leader — the shell that owns this
// terminal — goes away. A closing terminal is supposed to arrive as SIGHUP,
// but a pane killed out from under a process group that is no longer the
// terminal's foreground one produces no signal at all: the dev sessions found
// leaked on this machine were exactly that shape, still running with revoked
// stdio and nothing left to tell them. Watching the leader catches the close
// however it happens, and is the reason this does not depend on the signal
// path being reliable.
//
// A process that is its own session leader has no owning terminal to lose, so
// it never fires — which is also what keeps this quiet under `go test`.
func terminalClosed(logger zerolog.Logger) <-chan struct{} {
	closed := make(chan struct{})
	leader, err := unix.Getsid(0)
	if err != nil || leader == os.Getpid() {
		logger.Debug().Err(err).Msg("no owning terminal to watch")
		return closed
	}
	go watchProcess(leader, terminalInterval, closed)
	return closed
}

// watchProcess closes gone once pid is no longer running.
func watchProcess(pid int, interval time.Duration, gone chan<- struct{}) {
	for {
		if syscall.Kill(pid, syscall.Signal(0)) != nil {
			close(gone)
			return
		}
		time.Sleep(interval)
	}
}

// stopApp asks every app process under the runner to close and waits for it.
// Reaching the grace period is not fatal: the runner's kill is still coming,
// and a dev loop that refuses to quit because the app is wedged would be worse
// than one that loses a clean database close.
func stopApp(logger zerolog.Logger, tree []procEntry, urgent <-chan struct{}) {
	apps := make([]procEntry, 0, 1)
	for _, entry := range tree {
		if commandName(entry.command) == appProcessName {
			apps = append(apps, entry)
		}
	}
	if len(apps) == 0 {
		return
	}

	for _, app := range apps {
		logger.Info().Int("pid", app.pid).Msg("asking the app to shut down")
		if err := syscall.Kill(app.pid, syscall.SIGTERM); err != nil {
			logger.Debug().Err(err).Int("pid", app.pid).Msg("signalling the app")
		}
	}

	// Polled rather than waited on: these belong to the runner, not to this
	// process, so there is nothing to wait(2) for.
	alive := time.NewTicker(aliveInterval)
	defer alive.Stop()
	grace := time.NewTimer(appGrace)
	defer grace.Stop()

	for {
		select {
		case <-alive.C:
			if !anyAlive(apps) {
				logger.Debug().Msg("app shut down")
				return
			}
		case <-urgent:
			return
		case <-grace.C:
			logger.Warn().Dur("grace", appGrace).Msg("app did not exit after SIGTERM; the runner's kill will end it")
			return
		}
	}
}

// sweep force-kills whatever from the snapshot is still running. It re-reads
// the process table first and compares the command: the snapshot's pids were
// recorded before the teardown, and a pid recycled since then must not be
// killed for the process that used to hold it.
func sweep(logger zerolog.Logger, snapshot []procEntry) {
	table, err := processTable()
	if err != nil {
		logger.Warn().Err(err).Msg("skipping the leftover sweep")
		return
	}
	live := make(map[int]string, len(table))
	for _, entry := range table {
		live[entry.pid] = entry.command
	}
	for _, entry := range snapshot {
		if command, ok := live[entry.pid]; !ok || command != entry.command {
			continue
		}
		logger.Warn().Int("pid", entry.pid).Str("process", commandName(entry.command)).Msg("killing leftover dev process")
		if err := syscall.Kill(entry.pid, syscall.SIGKILL); err != nil {
			logger.Debug().Err(err).Int("pid", entry.pid).Msg("killing leftover dev process")
		}
	}
}

func anyAlive(entries []procEntry) bool {
	for _, entry := range entries {
		if syscall.Kill(entry.pid, syscall.Signal(0)) == nil {
			return true
		}
	}
	return false
}

// runnerExit reports the runner's own exit status as this command's, so a
// failed build or a runner that could not start still fails the mise task.
func runnerExit(err error) error {
	if err == nil {
		return nil
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return cli.Exit(fmt.Sprintf("dev runner exited with status %d", exit.ExitCode()), exit.ExitCode())
	}
	return err
}

// procEntry is one row of the process table: enough to walk the tree and to
// re-check identity before signalling.
type procEntry struct {
	pid     int
	ppid    int
	command string
}

// descendantsOf returns every process below root. A failure to read the table
// degrades to an empty tree — the runner's own teardown still runs, this only
// loses the graceful stop and the sweep.
func descendantsOf(logger zerolog.Logger, root int) []procEntry {
	table, err := processTable()
	if err != nil {
		logger.Warn().Err(err).Msg("reading the process tree; falling back to the runner's own teardown")
		return nil
	}
	return descendants(table, root)
}

func descendants(table []procEntry, root int) []procEntry {
	byParent := make(map[int][]procEntry, len(table))
	for _, entry := range table {
		byParent[entry.ppid] = append(byParent[entry.ppid], entry)
	}

	var found []procEntry
	for queue := []int{root}; len(queue) > 0; {
		pid := queue[0]
		queue = queue[1:]
		for _, child := range byParent[pid] {
			found = append(found, child)
			queue = append(queue, child.pid)
		}
	}
	return found
}

// processTable reads pid, ppid and command for every process. ps is the only
// portable reader here: macOS has no /proc, and the dev loop runs on both.
func processTable() ([]procEntry, error) {
	out, err := exec.Command("ps", "-eo", "pid=,ppid=,command=").Output() //nolint:noctx // bounded by ps itself
	if err != nil {
		return nil, fmt.Errorf("read process table: %w", err)
	}
	return parseProcessTable(string(out)), nil
}

func parseProcessTable(out string) []procEntry {
	var table []procEntry
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		table = append(table, procEntry{pid: pid, ppid: ppid, command: strings.Join(fields[2:], " ")})
	}
	return table
}

func commandName(command string) string {
	argv0, _, _ := strings.Cut(command, " ")
	return filepath.Base(argv0)
}
