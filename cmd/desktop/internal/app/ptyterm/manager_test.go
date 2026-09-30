package ptyterm

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The shell every test spawns: `sh` reads a line and echoes it back, which is
// all these tests need and is present on every platform the package supports.
func testManager(t *testing.T) *Manager {
	t.Helper()
	if !platformSupported() {
		t.Skip("PTYs are unavailable on this build or platform")
	}
	m := NewManager(ManagerOptions{Shell: []string{"/bin/sh"}})
	t.Cleanup(func() { _ = m.Stop(t.Context()) })
	return m
}

// awaitOutput reads the stream until the accumulated bytes contain want, and
// fails with what it did see instead.
func awaitOutput(t *testing.T, events <-chan Event, want string) {
	t.Helper()
	var seen strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("stream closed before %q; saw %q", want, seen.String())
			}
			if out, isOutput := ev.(Output); isOutput {
				seen.Write(out.Data)
				if strings.Contains(seen.String(), want) {
					return
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q; saw %q", want, seen.String())
		}
	}
}

func awaitExit(t *testing.T, events <-chan Event) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("stream closed before the exit notice")
			}
			if _, exited := ev.(Exited); exited {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for the exit notice")
		}
	}
}

func TestOpenAndEcho(t *testing.T) {
	m := testManager(t)
	dir := t.TempDir()

	term, err := m.Open(t.Context(), Spec{Dir: dir, Cols: 100, Rows: 30})
	require.NoError(t, err)
	require.Equal(t, dir, term.Dir)
	require.Equal(t, 100, term.Cols)
	require.Equal(t, 30, term.Rows)
	require.Equal(t, "sh", term.Title)
	require.Equal(t, []Terminal{term}, m.List())

	events, unsubscribe, err := m.Subscribe(term.ID)
	require.NoError(t, err)
	defer unsubscribe()

	require.NoError(t, m.Write(term.ID, []byte("echo hive-pty-ok\n")))
	awaitOutput(t, events, "hive-pty-ok")
}

// Each open is its own terminal: nothing is keyed on the directory or the
// command, so a second pop-up over the same checkout is a second shell.
func TestOpenIsAlwaysANewTerminal(t *testing.T) {
	m := testManager(t)
	dir := t.TempDir()

	first, err := m.Open(t.Context(), Spec{Dir: dir})
	require.NoError(t, err)
	second, err := m.Open(t.Context(), Spec{Dir: dir})
	require.NoError(t, err)

	require.NotEqual(t, first.ID, second.ID)
	require.Len(t, m.List(), 2)
}

// A launcher's command is a command line, not an argv: it runs through the
// shell so a user's aliases, functions and PATH are what resolve it.
func TestCommandRunsThroughTheShell(t *testing.T) {
	m := testManager(t)

	// `cat` blocks on the PTY so the terminal outlives the subscribe. Without
	// it the shell exits in a millisecond, the manager forgets the terminal,
	// and Subscribe loses the race on a loaded machine.
	term, err := m.Open(t.Context(), Spec{Dir: t.TempDir(), Command: "echo one && echo two && cat"})
	require.NoError(t, err)
	require.Equal(t, "echo", term.Title)

	events, unsubscribe, err := m.Subscribe(term.ID)
	require.NoError(t, err)
	defer unsubscribe()

	awaitOutput(t, events, "two")
}

func TestResizeReachesTheProcess(t *testing.T) {
	m := testManager(t)

	term, err := m.Open(t.Context(), Spec{Dir: t.TempDir()})
	require.NoError(t, err)

	events, unsubscribe, err := m.Subscribe(term.ID)
	require.NoError(t, err)
	defer unsubscribe()

	require.NoError(t, m.Resize(term.ID, 132, 43))
	// The shell reads the size from the tty rather than from anything we told
	// it, so this asserts the ioctl landed, not that our bookkeeping agrees.
	require.NoError(t, m.Write(term.ID, []byte("stty size\n")))
	awaitOutput(t, events, "43 132")
}

// An exited terminal is gone rather than kept as a dead tab, so a pop-up whose
// command finished cannot be written to or listed.
func TestExitForgetsTheTerminal(t *testing.T) {
	m := testManager(t)

	term, err := m.Open(t.Context(), Spec{Dir: t.TempDir()})
	require.NoError(t, err)

	events, unsubscribe, err := m.Subscribe(term.ID)
	require.NoError(t, err)
	defer unsubscribe()

	require.NoError(t, m.Write(term.ID, []byte("exit\n")))
	awaitExit(t, events)
	require.Eventually(t, func() bool { return len(m.List()) == 0 }, 5*time.Second, 20*time.Millisecond)
	require.ErrorIs(t, m.Write(term.ID, []byte("ignored\n")), ErrNotFound)
}

// Re-subscribing replays the ring, which is what lets the panel be dismissed
// and brought back without blanking the shell behind it.
func TestResubscribeReplaysScrollback(t *testing.T) {
	m := testManager(t)

	term, err := m.Open(t.Context(), Spec{Dir: t.TempDir()})
	require.NoError(t, err)

	events, unsubscribe, err := m.Subscribe(term.ID)
	require.NoError(t, err)
	require.NoError(t, m.Write(term.ID, []byte("echo remembered-line\n")))
	awaitOutput(t, events, "remembered-line")
	unsubscribe()

	replayed, unsubscribe2, err := m.Subscribe(term.ID)
	require.NoError(t, err)
	defer unsubscribe2()
	awaitOutput(t, replayed, "remembered-line")
}

func TestCloseEndsTheTerminal(t *testing.T) {
	m := testManager(t)

	term, err := m.Open(t.Context(), Spec{Dir: t.TempDir()})
	require.NoError(t, err)

	closed, err := m.Close(term.ID)
	require.NoError(t, err)
	require.True(t, closed)
	require.Empty(t, m.List())

	closed, err = m.Close(term.ID)
	require.NoError(t, err)
	require.False(t, closed, "an id with no terminal is success with nothing done")
}

func TestOpenRejectsADirectoryItCannotEnter(t *testing.T) {
	m := testManager(t)

	_, err := m.Open(t.Context(), Spec{Dir: ""})
	require.ErrorIs(t, err, ErrInvalidSpec)

	_, err = m.Open(t.Context(), Spec{Dir: t.TempDir() + "/nope"})
	require.ErrorIs(t, err, ErrInvalidSpec)
}

func TestSizeBounds(t *testing.T) {
	m := testManager(t)

	_, err := m.Open(t.Context(), Spec{Dir: t.TempDir(), Cols: 80, Rows: maxDimension + 1})
	require.ErrorIs(t, err, ErrInvalidSize)

	term, err := m.Open(t.Context(), Spec{Dir: t.TempDir()})
	require.NoError(t, err, "an unmeasured size opens at the default")
	require.Equal(t, defaultCols, term.Cols)
	require.Equal(t, defaultRows, term.Rows)

	require.ErrorIs(t, m.Resize(term.ID, 0, 24), ErrInvalidSize)
	require.ErrorIs(t, m.Resize(term.ID, 80, maxDimension+1), ErrInvalidSize)
}

// A caller that already knows the id it wants to reattach with gets exactly
// that id back, not a minted one.
func TestOpenHonoursACallerSuppliedID(t *testing.T) {
	m := testManager(t)

	term, err := m.Open(t.Context(), Spec{ID: "workspace-session-1", Dir: t.TempDir()})
	require.NoError(t, err)
	require.Equal(t, "workspace-session-1", term.ID)
	require.Equal(t, []Terminal{term}, m.List())
}

// A second Open for an id already live is a rejection, not a second terminal —
// the caller's id is what it will reattach with, so a collision must not spawn
// a second process silently claiming it.
func TestOpenRejectsADuplicateID(t *testing.T) {
	m := testManager(t)

	first, err := m.Open(t.Context(), Spec{ID: "dup", Dir: t.TempDir()})
	require.NoError(t, err)

	_, err = m.Open(t.Context(), Spec{ID: "dup", Dir: t.TempDir()})
	require.ErrorIs(t, err, ErrIDInUse)

	require.Equal(t, []Terminal{first}, m.List(), "the rejected Open spawned nothing")
}

// A mint must never be able to collide with an address a caller is holding, so
// the minted shape is off-limits as a caller-supplied id.
func TestCallerIDCannotCollideWithAMintedID(t *testing.T) {
	m := testManager(t)

	for _, id := range []string{"t1", "t2", "t99", "t0"} {
		_, err := m.Open(t.Context(), Spec{ID: id, Dir: t.TempDir()})
		require.ErrorIs(t, err, ErrInvalidID, "id %q", id)
	}
	require.Empty(t, m.List())
}

func TestValidateIDBounds(t *testing.T) {
	require.ErrorIs(t, validateID(""), ErrInvalidID, "empty")
	require.ErrorIs(t, validateID(strings.Repeat("a", 65)), ErrInvalidID, "65 characters")
	require.ErrorIs(t, validateID("has/slash"), ErrInvalidID, "slash")
	require.ErrorIs(t, validateID("has..dots"), ErrInvalidID, "..")
	require.ErrorIs(t, validateID("has space"), ErrInvalidID, "space")
	require.ErrorIs(t, validateID("has\x00nul"), ErrInvalidID, "NUL")

	require.NoError(t, validateID(strings.Repeat("a", 64)), "64 characters is in bounds")
}

// The cap is the cheapest bound that stops an enthusiastic afternoon from
// being a fork bomb with a progress bar: the ninth terminal is refused before
// a process is spawned, and closing one makes room for the next.
func TestOpenCapsConcurrentTerminals(t *testing.T) {
	m := testManager(t)
	dir := t.TempDir()

	var opened []Terminal
	for range maxConcurrentSessions {
		term, err := m.Open(t.Context(), Spec{Dir: dir})
		require.NoError(t, err)
		opened = append(opened, term)
	}

	_, err := m.Open(t.Context(), Spec{Dir: dir})
	require.ErrorIs(t, err, ErrTooManyTerminals)
	require.Len(t, m.List(), maxConcurrentSessions, "the rejected Open spawned nothing")

	closed, err := m.Close(opened[0].ID)
	require.NoError(t, err)
	require.True(t, closed)

	_, err = m.Open(t.Context(), Spec{Dir: dir})
	require.NoError(t, err, "closing one makes room for the next")
}
