//go:build !server

package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/domain/session"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/platform/tmux/control"
	"github.com/colonyops/hive/internal/platform/tmuxtest"
)

// These tests drive a real tmux server: whether to attach or to offer a start
// is a decision about what tmux is holding, and a faked one would only prove the
// fake. See internal/platform/tmuxtest for how they are kept away from a developer's own.

func newTestTerminals(t *testing.T, runner tmuxcc.Runner, starter terminalStarter) *TerminalsService {
	t.Helper()
	return newTestTerminalsIn(t, runner, starter, os.UserHomeDir)
}

// newTestTerminalsIn is newTestTerminals with the home directory the scratch
// terminal opens in, so a test can assert where it landed without opening one in
// the developer's own home.
func newTestTerminalsIn(t *testing.T, runner tmuxcc.Runner, starter terminalStarter, home func() (string, error)) *TerminalsService {
	t.Helper()
	return newTestTerminalsWith(t, runner, starter, home, zerolog.Nop())
}

// newTestTerminalsWith is newTestTerminalsIn with the logger a test wants to
// read back, for asserting what a failure logs.
func newTestTerminalsWith(t *testing.T, runner tmuxcc.Runner, starter terminalStarter, home func() (string, error), logger zerolog.Logger) *TerminalsService {
	t.Helper()
	manager := tmuxcc.NewManager(t.Context(), tmuxcc.ManagerOptions{Logger: zerolog.Nop(), Runner: runner})
	t.Cleanup(func() { _ = manager.Stop(context.WithoutCancel(t.Context())) })
	return newTerminalsService(TerminalsDeps{Manager: manager, Starter: starter, Home: home, Logger: logger})
}

// spawningStarter stands in for the session service: it creates the tmux session
// the way hive's spawn would, and records that it was asked to.
type spawningStarter struct {
	tmux  func(args ...string) error
	calls []string
	err   error
}

func (s *spawningStarter) StartTmuxSession(_ context.Context, slug string) (multiplexer.LaunchResult, error) {
	s.calls = append(s.calls, slug)
	if s.err != nil {
		return multiplexer.LaunchResult{}, s.err
	}
	return multiplexer.LaunchResult{Created: true}, s.tmux("new-session", "-d", "-s", slug, "-n", "claude", "sh")
}

func TestTerminalsStartThenAttachIsTheColdPath(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	starter := &spawningStarter{tmux: tmux}
	terminals := newTestTerminals(t, server, starter)

	// Attaching does not spawn: a session that was never started reports itself
	// so the caller can offer to start it, and nothing runs an agent behind the
	// user's back.
	_, err := terminals.Attach(t.Context(), "hive-cold", 120, 40)
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.Empty(t, starter.calls)

	started, err := terminals.Start(t.Context(), "hive-cold")
	require.NoError(t, err)
	assert.True(t, started.Created, "this call is what created the session")
	assert.Equal(t, []string{"hive-cold"}, starter.calls)

	windows, err := terminals.Attach(t.Context(), "hive-cold", 120, 40)
	require.NoError(t, err)
	require.Len(t, windows, 1, "the attach lands on the session the start spawned")
	assert.Equal(t, "claude", windows[0].Name)
}

func TestTerminalsStartLeavesALiveSessionAlone(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	require.NoError(t, tmux("-f", "/dev/null", "new-session", "-d", "-s", "hive-live", "-n", "claude", "-x", "120", "-y", "40", "sh"))
	starter := &spawningStarter{tmux: tmux}
	terminals := newTestTerminals(t, server, starter)

	started, err := terminals.Start(t.Context(), "hive-live")
	require.NoError(t, err)
	assert.False(t, started.Created, "a session tmux is already running is never spawned over")
	assert.Empty(t, starter.calls)

	windows, err := terminals.Attach(t.Context(), "hive-live", 120, 40)
	require.NoError(t, err)
	require.Len(t, windows, 1)
}

func TestTerminalsKillEndsTheSessionAndLeavesItStartableAgain(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	require.NoError(t, tmux("-f", "/dev/null", "new-session", "-d", "-s", "hive-kill", "-n", "claude", "-x", "120", "-y", "40", "sh"))
	starter := &spawningStarter{tmux: tmux}
	terminals := newTestTerminals(t, server, starter)

	_, err := terminals.Attach(t.Context(), "hive-kill", 120, 40)
	require.NoError(t, err)

	killed, err := terminals.Kill(t.Context(), "hive-kill")
	require.NoError(t, err)
	assert.True(t, killed)

	// The attach that follows sees a session that is not running rather than a
	// stale client, which is what puts the view back on its start panel.
	_, err = terminals.Attach(t.Context(), "hive-kill", 120, 40)
	assert.Equal(t, KindNotFound, KindOf(err))

	// Killing what is already gone is nothing to do, not a failure.
	killed, err = terminals.Kill(t.Context(), "hive-kill")
	require.NoError(t, err)
	assert.False(t, killed)

	started, err := terminals.Start(t.Context(), "hive-kill")
	require.NoError(t, err)
	assert.True(t, started.Created, "a killed session can be started again")
}

// Reordering is the one operation whose result is not what the caller asked
// for: tmux owns the order, so the service answers with what tmux settled on.
func TestTerminalsMoveWindowReordersAndKeepsTheSelection(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	require.NoError(t, tmux("-f", "/dev/null", "new-session", "-d", "-s", "hive-move", "-n", "alpha", "-x", "120", "-y", "40", "sh"))
	require.NoError(t, tmux("new-window", "-t", "hive-move", "-n", "bravo", "sh"))
	require.NoError(t, tmux("new-window", "-t", "hive-move", "-n", "charlie", "sh"))
	terminals := newTestTerminals(t, server, &spawningStarter{tmux: tmux})

	windows, err := terminals.Attach(t.Context(), "hive-move", 120, 40)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "bravo", "charlie"}, windowNames(windows))
	active := activeWindowID(t, windows)

	moved, err := terminals.MoveWindow(t.Context(), "hive-move", windows[0].ID, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"bravo", "charlie", "alpha"}, windowNames(moved))
	assert.Equal(t, active, activeWindowID(t, moved), "a reorder is not a selection")

	// Indices are what tmux's own key bindings and every other attached client
	// address a window by, so an insert must not leave them with holes.
	listed, err := terminals.ListAllWindows(t.Context(), []string{"hive-move"})
	require.NoError(t, err)
	assert.Equal(t, windowNames(moved), windowNames(listed["hive-move"]))
	// The fixture's server runs with no config, so the run starts at tmux's own
	// base-index of 0.
	assert.Equal(t, "0 1 2", tmuxFields(t, server, "list-windows", "-t", "hive-move", "-F", "#{window_index}"))
}

func TestTerminalsMoveWindowClassifiesWhatItRefuses(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	require.NoError(t, tmux("-f", "/dev/null", "new-session", "-d", "-s", "hive-badmove", "-n", "alpha", "-x", "120", "-y", "40", "sh"))
	terminals := newTestTerminals(t, server, &spawningStarter{tmux: tmux})

	_, err := terminals.MoveWindow(t.Context(), "hive-badmove", "@0", 0)
	assert.Equal(t, KindNotFound, KindOf(err), "no client is attached for that slug")

	windows, err := terminals.Attach(t.Context(), "hive-badmove", 120, 40)
	require.NoError(t, err)

	_, err = terminals.MoveWindow(t.Context(), "hive-badmove", "@404", 0)
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = terminals.MoveWindow(t.Context(), "hive-badmove", windows[0].ID, 7)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func windowNames(windows []tmuxcc.Window) []string {
	names := make([]string, 0, len(windows))
	for _, w := range windows {
		names = append(names, w.Name)
	}
	return names
}

func activeWindowID(t *testing.T, windows []tmuxcc.Window) string {
	t.Helper()
	for _, w := range windows {
		if w.Active {
			return w.ID
		}
	}
	t.Fatalf("no active window in %#v", windows)
	return ""
}

// tmuxFields reads a tmux listing as one space-joined line, so a per-row format
// reads as the sequence it describes.
func tmuxFields(t *testing.T, server *tmuxtest.Server, args ...string) string {
	t.Helper()
	out, err := server.Output(args...)
	require.NoError(t, err)
	return strings.Join(strings.Fields(out), " ")
}

// The scratch terminal is the one session the desktop creates itself: hive has
// no record to spawn from, so what it gets instead is the user's home directory.
func TestTerminalsStartScratchOpensItInHomeWithoutAskingHive(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	home := t.TempDir()
	starter := &spawningStarter{tmux: tmux}
	terminals := newTestTerminalsIn(t, server, starter, func() (string, error) { return home, nil })

	started, err := terminals.Start(t.Context(), ScratchSlug)
	require.NoError(t, err)
	assert.True(t, started.Created)
	assert.Empty(t, starter.calls, "there is no hive session to spawn from")

	// The home directory is the *session's* working directory rather than the
	// first window's, which is what makes every tab opened later start there too.
	assert.Equal(t, realpath(t, home), realpath(t, tmuxFields(t, server, "display-message", "-p", "-t", ScratchSlug, "#{session_path}")))

	// Nothing else about it is special: the slug is the whole contract, so the
	// attach and the window it opens are the ones every session uses.
	windows, err := terminals.Attach(t.Context(), ScratchSlug, 120, 40)
	require.NoError(t, err)
	require.Len(t, windows, 1)

	tab, err := terminals.NewWindow(t.Context(), ScratchSlug)
	require.NoError(t, err)
	assertPaneIn(t, server, tab, home)

	started, err = terminals.Start(t.Context(), ScratchSlug)
	require.NoError(t, err)
	assert.False(t, started.Created, "a scratch terminal that is running is never recreated over")
}

// An agent workspace chat is a tmux session in its own namespace (ADR
// agent-workspace-sessions-are-tmux-sessions), and the Code view now attaches
// pinned ones through this service. That works only because the slug is the whole
// contract here — nothing on the attach or sweep path checks it against hive's
// session list — so this pins the property the sidebar depends on, including the
// half that must *not* work: Start would ask hive for a spawn configuration that
// does not exist, which is why resuming a chat is the Agents area's own call.
func TestTerminalsAttachAndSweepAnAgentChatSlugHiveKnowsNothingAbout(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	const slug = "agentws-42"
	require.NoError(t, tmux("-f", "/dev/null", "new-session", "-d", "-s", slug, "-n", "claude", "-x", "120", "-y", "40", "sh"))
	starter := &spawningStarter{err: errors.New("no such hive session")}
	terminals := newTestTerminals(t, server, starter)

	windows, err := terminals.Attach(t.Context(), slug, 120, 40)
	require.NoError(t, err)
	require.Len(t, windows, 1)
	assert.Equal(t, "claude", windows[0].Name)
	assert.Empty(t, starter.calls, "attaching never goes through hive's spawn")

	// The sidebar reads a pinned chat's liveness off this sweep, because hive's
	// status projection has no row to answer for it.
	swept, err := terminals.ListAllWindows(t.Context(), []string{slug})
	require.NoError(t, err)
	assert.Len(t, swept[slug], 1)

	// A chat tmux is not holding reports itself the same way any cold session
	// does, which is what puts the pane into its offer-a-resume state.
	_, err = terminals.Attach(t.Context(), "agentws-43", 120, 40)
	assert.Equal(t, KindNotFound, KindOf(err))
}

// The + on a row is offered for the session, not for what is on screen, so it
// has to work before anything has attached — which is what a first click on the
// pinned terminal is.
func TestTerminalsNewWindowWithoutAnAttachOpensWhereTheSessionIs(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	home := t.TempDir()
	terminals := newTestTerminalsIn(t, server, &spawningStarter{tmux: tmux}, func() (string, error) { return home, nil })

	_, err := terminals.Start(t.Context(), ScratchSlug)
	require.NoError(t, err)

	// No Attach in between: the window is made by a one-shot, and it lands where
	// the session lives rather than in this process's working directory.
	id, err := terminals.NewWindow(t.Context(), ScratchSlug)
	require.NoError(t, err)
	assertPaneIn(t, server, id, home)

	windows, err := terminals.ListAllWindows(t.Context(), []string{ScratchSlug})
	require.NoError(t, err)
	assert.Len(t, windows[ScratchSlug], 2)
}

// A new tab opens where the terminal it was asked for is, which is what makes
// it different from where the session was started: the scratch terminal starts
// in the user's home and its tabs are wherever their panes have gone since.
//
// The pane is moved with tmux's own -c rather than by typing `cd` into a shell.
// What is under test is which pane the directory is read from, not whether a
// prompt followed a keystroke, and a test that waits for a shell is a test that
// is flaky for reasons of its own.
func TestTerminalsNewWindowFollowsTheActivePane(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	home := t.TempDir()
	elsewhere := t.TempDir()
	terminals := newTestTerminalsIn(t, server, &spawningStarter{tmux: tmux}, func() (string, error) { return home, nil })

	_, err := terminals.Start(t.Context(), ScratchSlug)
	require.NoError(t, err)
	require.NoError(t, tmux("new-window", "-t", ScratchSlug, "-c", elsewhere, "-n", "elsewhere", "sh"))

	// Unattached, through the one-shot: tmux resolves the slug to the session's
	// current window, which is the one it just made.
	id, err := terminals.NewWindow(t.Context(), ScratchSlug)
	require.NoError(t, err)
	assertPaneIn(t, server, id, elsewhere)

	// Attached, over the control stream: the same answer, read from the client's
	// own current window rather than from a target it was handed.
	_, err = terminals.Attach(t.Context(), ScratchSlug, 120, 40)
	require.NoError(t, err)
	attached, err := terminals.NewWindow(t.Context(), ScratchSlug)
	require.NoError(t, err)
	assertPaneIn(t, server, attached, elsewhere)
}

// WorkingDirectory is what a launcher with no cwd opens in, and the whole point
// of asking tmux rather than hive is that the scratch terminal has no hive
// record to ask about.
func TestTerminalsWorkingDirectoryIsTheActivePanes(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	home := t.TempDir()
	elsewhere := t.TempDir()
	terminals := newTestTerminalsIn(t, server, &spawningStarter{tmux: tmux}, func() (string, error) { return home, nil })

	_, err := terminals.Start(t.Context(), ScratchSlug)
	require.NoError(t, err)

	assertWorkingDirectory(t, terminals, home)

	require.NoError(t, tmux("new-window", "-t", ScratchSlug, "-c", elsewhere, "-n", "elsewhere", "sh"))
	assertWorkingDirectory(t, terminals, elsewhere)
}

// A terminal that is not running has no pane to read. It is KindNotFound rather
// than a directory of last resort, because what to fall back to belongs to the
// caller: a hive session still has its checkout, and a scratch terminal has
// nothing.
func TestTerminalsWorkingDirectoryInASessionThatIsNotRunning(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	terminals := newTestTerminals(t, server, &spawningStarter{})

	_, err := terminals.WorkingDirectory(t.Context(), "hive-gone")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestTerminalsNewWindowInASessionThatIsNotRunning(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	terminals := newTestTerminals(t, server, &spawningStarter{})

	_, err := terminals.NewWindow(t.Context(), "hive-gone")
	// A session to add a window to is the caller's to create — a start for a
	// hive session runs its agent, so this must not do it on their behalf.
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestTerminalsNewWindowInASessionThatIsNotRunningLogsTheSlug(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	var logs bytes.Buffer
	terminals := newTestTerminalsWith(t, server, &spawningStarter{}, os.UserHomeDir, zerolog.New(&logs))

	_, err := terminals.NewWindow(t.Context(), "hive-gone")

	require.Error(t, err)
	assert.Contains(t, logs.String(), "hive-gone")
}

func TestTerminalsStartScratchReportsAnUnreadableHome(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	terminals := newTestTerminalsIn(t, server, &spawningStarter{}, func() (string, error) { return "", errors.New("no home") })

	_, err := terminals.Start(t.Context(), ScratchSlug)
	assert.Equal(t, KindUnavailable, KindOf(err))
}

// The scratch session lives in tmux's one session namespace alongside hive's, so
// the slug it claims has to be one hive cannot mint — otherwise a session named
// "Scratch" would silently become the scratch terminal, or take it over.
func TestScratchSlugIsUnreachableFromASessionName(t *testing.T) {
	t.Parallel()
	terminals := newTestTerminals(t, nil, nil)
	assert.Equal(t, ScratchSlug, terminals.Scratch(t.Context()).Slug)

	for _, name := range []string{"Scratch", "scratch", "SCRATCH", "  Scratch  ", "scratch/1"} {
		assert.NotEqual(t, ScratchSlug, session.Slugify(name),
			"a hive session named %q must not slugify onto the scratch terminal", name)
	}
}

func realpath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

func TestTerminalsStartReportsWhyItCouldNot(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	starter := &spawningStarter{tmux: tmux, err: Errorf(KindNotFound, "no session named %q", "hive-gone")}
	terminals := newTestTerminals(t, server, starter)

	_, err := terminals.Start(t.Context(), "hive-gone")
	// The starter's classification survives: a start that failed because nothing
	// knows the slug must not read as a tmux fault.
	assert.Equal(t, KindNotFound, KindOf(err))
	assert.Contains(t, err.Error(), "hive-gone")
}

// Whether a tab closes silently is a question about processes: a shell waiting
// at its prompt is idle, and anything running in front of it is not — including
// the two cases either half of the check would get wrong on its own.
func TestTerminalsWindowForegroundTellsAPromptFromWork(t *testing.T) {
	t.Parallel()
	server := tmuxtest.New(t)
	tmux := server.Run
	terminals := newTestTerminals(t, server, &spawningStarter{tmux: tmux})
	require.NoError(t, tmux("new-session", "-d", "-s", "hive-fg", "-n", "shell", "-x", "120", "-y", "40"))
	// tmux runs a window's command through sh -c, which execs it in place: the
	// pane's own process becomes the work, so nothing it started is running and
	// the process check alone would read this as a prompt.
	require.NoError(t, tmux("new-window", "-t", "hive-fg", "-n", "agent", "sleep 300"))

	windows, err := terminals.Attach(t.Context(), "hive-fg", 120, 40)
	require.NoError(t, err)
	byName := map[string]string{}
	for _, window := range windows {
		byName[window.Name] = window.ID
	}
	require.Len(t, byName, 2)

	awaitForeground(t, terminals, "hive-fg", byName["agent"], WindowForeground{Running: true, Command: "sleep"})
	awaitForeground(t, terminals, "hive-fg", byName["shell"], WindowForeground{})

	// The other half: a shell script runs under its interpreter's own name, so
	// the name alone reads as a prompt. The pane's process is what says
	// otherwise — the shell is waiting on something rather than on the user.
	// bash, because macOS reports its sh under bash's name.
	require.NoError(t, tmux("send-keys", "-t", "hive-fg:shell", "bash -c 'sleep 300; true'", "Enter"))
	awaitForeground(t, terminals, "hive-fg", byName["shell"], WindowForeground{Running: true, Command: "bash"})
}

// awaitForeground polls until the window answers want, because the answer is
// whatever the pane's process tree is doing at the moment it is read: the shell
// tmux started has to reach its prompt, and a command it was sent has to be
// forked *and* exec'd — between those two the pane is already running something
// under the name of the shell that started it.
func awaitForeground(t *testing.T, terminals *TerminalsService, slug, windowID string, want WindowForeground) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last WindowForeground
	for time.Now().Before(deadline) {
		foreground, err := terminals.WindowForeground(t.Context(), slug, windowID)
		require.NoError(t, err)
		last = foreground
		if foreground == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("window %s never answered %+v; its last answer was %+v", windowID, want, last)
}

// What a window answers with is the whole window's, because closing one kills
// every pane in it — and an answer it could not establish is work, not a prompt.
func TestTerminalsForegroundOfReadsTheWholeWindow(t *testing.T) {
	t.Parallel()

	atPrompt := map[int]bool{100: true, 200: true}
	terminals := &TerminalsService{foreground: func(_ context.Context, pid int) (bool, error) {
		if pid == 0 {
			return false, errors.New("no such process")
		}
		return atPrompt[pid], nil
	}}

	cases := map[string]struct {
		panes []tmuxcc.Pane
		want  WindowForeground
	}{
		"every pane at a prompt": {
			panes: []tmuxcc.Pane{
				{ID: "%1", PID: 100, Active: true, Command: "zsh"},
				{ID: "%2", PID: 200, Command: "-bash"},
			},
		},
		"the active pane's process is the one named": {
			panes: []tmuxcc.Pane{
				{ID: "%1", PID: 300, Command: "npm"},
				{ID: "%2", PID: 400, Active: true, Command: "claude"},
			},
			want: WindowForeground{Running: true, Command: "claude"},
		},
		"a background pane alone is enough": {
			panes: []tmuxcc.Pane{
				{ID: "%1", PID: 100, Active: true, Command: "zsh"},
				{ID: "%2", PID: 300, Command: "nvim"},
			},
			want: WindowForeground{Running: true, Command: "nvim"},
		},
		"a dead pane is not running anything": {
			panes: []tmuxcc.Pane{{ID: "%1", Active: true, Dead: true, Command: "claude"}},
		},
		"a process that cannot be read is work": {
			panes: []tmuxcc.Pane{{ID: "%1", PID: 0, Active: true, Command: "zsh"}},
			want:  WindowForeground{Running: true, Command: "zsh"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, terminals.foregroundOf(t.Context(), "hive-abc", tc.panes))
		})
	}
}

func TestTerminals_ServerPaneNeverReadsAsAPrompt(t *testing.T) {
	t.Parallel()
	terminals := &TerminalsService{foreground: func(context.Context, int) (bool, error) {
		t.Error("a server's pid must not be looked up on this machine")
		return true, nil
	}}

	got := terminals.foregroundOf(t.Context(), "box:hive-abc", []tmuxcc.Pane{{ID: "%1", PID: 42, Active: true, Command: "zsh"}})

	assert.Equal(t, WindowForeground{Running: true, Command: "zsh"}, got)
}

// tmux reads a pane's path from its process's cwd, and a pane that was just
// forked keeps the server's cwd until its chdir runs. Read once, a slow runner
// sees the tmuxtest home instead of the pane's directory.
func assertPaneIn(t *testing.T, server *tmuxtest.Server, pane, dir string) {
	t.Helper()
	want := realpath(t, dir)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := server.Output("display-message", "-p", "-t", pane, "#{pane_current_path}")
		require.NoError(c, err)
		got, err := filepath.EvalSymlinks(strings.TrimSpace(out))
		require.NoError(c, err)
		assert.Equal(c, want, got)
	}, 5*time.Second, 20*time.Millisecond)
}

// assertWorkingDirectory waits for the reason assertPaneIn gives.
func assertWorkingDirectory(t *testing.T, terminals *TerminalsService, dir string) {
	t.Helper()
	want := realpath(t, dir)
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		got, err := terminals.WorkingDirectory(t.Context(), ScratchSlug)
		require.NoError(c, err)
		resolved, err := filepath.EvalSymlinks(got)
		require.NoError(c, err)
		assert.Equal(c, want, resolved)
	}, 5*time.Second, 20*time.Millisecond)
}
