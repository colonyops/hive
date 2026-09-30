package ptyterm

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
)

const (
	// readChunk is one PTY read. It is the latency/syscall trade: a read returns
	// as soon as the kernel has anything, so this only caps a burst.
	readChunk = 32 << 10
	// hangupGrace is how long a closed master gets to make the process exit on
	// its own before it is killed outright. Closing first is what lets a shell
	// hang up its own jobs; killing first orphans them.
	hangupGrace = 250 * time.Millisecond
)

// Terminal is what a caller sees of a live terminal. Command is the command
// line it was opened with, empty for a plain interactive shell.
type Terminal struct {
	ID      string
	Title   string
	Dir     string
	Command string
	Cols    int
	Rows    int
}

// spawnOptions is one terminal's spawn. Argv is what actually runs; Command is
// what was asked for, kept only so a caller can see it.
type spawnOptions struct {
	ID      string
	Title   string
	Dir     string
	Command string
	Argv    []string
	Env     []string
	Cols    int
	Rows    int
}

// terminal is one live terminal: a PTY master, the process on the far end of
// it, the replay ring for what it has printed, and the single subscriber
// streaming it.
type terminal struct {
	id      string
	title   string
	dir     string
	command string

	file   *os.File
	cmd    *exec.Cmd
	ring   *ring
	events *broker

	// publishMu orders ring writes against replay snapshots, so a chunk is
	// either in the snapshot a subscriber opens with or on the stream after it,
	// never both and never out of order.
	publishMu sync.Mutex

	mu     sync.Mutex
	cols   int
	rows   int
	exited bool
	reason string

	readerDone chan struct{}
	closeOnce  sync.Once
}

// spawn starts a terminal's command on a new PTY sized to opts. onExit fires
// once, after the reader has drained everything the process wrote.
func spawn(opts spawnOptions, onExit func(id string)) (*terminal, error) {
	if len(opts.Argv) == 0 {
		return nil, fmt.Errorf("%w: no command", ErrInvalidSpec)
	}
	cmd := exec.Command(opts.Argv[0], opts.Argv[1:]...) //nolint:noctx // the child outlives the request; it is closed explicitly
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env

	size := &pty.Winsize{Cols: uint16(opts.Cols), Rows: uint16(opts.Rows)} //nolint:gosec // bounded by validateSize
	file, err := pty.StartWithSize(cmd, size)
	if err != nil {
		return nil, fmt.Errorf("ptyterm: start %s: %w", opts.Argv[0], err)
	}

	t := &terminal{
		id:         opts.ID,
		title:      opts.Title,
		dir:        opts.Dir,
		command:    opts.Command,
		file:       file,
		cmd:        cmd,
		ring:       newRing(defaultReplayBytes),
		events:     newBroker(defaultBufferBytes),
		cols:       opts.Cols,
		rows:       opts.Rows,
		readerDone: make(chan struct{}),
	}
	go t.read(onExit)
	return t, nil
}

// read drains the PTY for the life of the process. Draining continues whether
// or not anything is subscribed: the far end blocks on a full PTY buffer, so a
// terminal nobody is watching would stop the process inside it.
func (t *terminal) read(onExit func(string)) {
	defer close(t.readerDone)

	buf := make([]byte, readChunk)
	for {
		n, err := t.file.Read(buf)
		if n > 0 {
			t.publish(buf[:n])
		}
		if err != nil {
			t.finish(exitReason(t.cmd))
			t.events.Publish(Exited{Reason: t.exitReason()})
			onExit(t.id)
			return
		}
	}
}

// publish records a chunk in the replay ring and puts it on the stream as one
// step, so a subscriber that opens mid-burst sees each byte exactly once.
func (t *terminal) publish(data []byte) {
	t.publishMu.Lock()
	defer t.publishMu.Unlock()
	_, _ = t.ring.Write(data)
	t.events.Publish(Output{Data: data})
}

// subscribe opens the event stream and replays the ring behind it, which is the
// first paint: a transport that just connected sees the screen as it stands
// before it sees anything new.
func (t *terminal) subscribe() (<-chan Event, func()) {
	t.publishMu.Lock()
	defer t.publishMu.Unlock()

	ch, unsubscribe := t.events.Subscribe()
	if snapshot := t.ring.Snapshot(); len(snapshot) > 0 {
		t.events.Publish(Output{Data: snapshot})
	}
	// The process can exit between the manager handing this terminal out and
	// the subscription landing; without this the transport would wait on a
	// stream nothing will ever write to again.
	if reason := t.exitReason(); reason != "" {
		t.events.Publish(Exited{Reason: reason})
	}
	return ch, unsubscribe
}

// exitReason answers why a terminal's process left. The read error that ends a
// PTY says nothing about it — a closed master reports EIO rather than EOF on
// Linux — so the process's own wait status carries the answer.
func exitReason(cmd *exec.Cmd) string {
	waitErr := cmd.Wait()
	var exit *exec.ExitError
	switch {
	case errors.As(waitErr, &exit):
		return fmt.Sprintf("exited with status %d", exit.ExitCode())
	case waitErr != nil:
		return waitErr.Error()
	default:
		return "exited"
	}
}

func (t *terminal) finish(reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.exited {
		return
	}
	t.exited = true
	t.reason = reason
}

func (t *terminal) exitReason() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reason
}

func (t *terminal) hasExited() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exited
}

func (t *terminal) write(p []byte) error {
	if t.hasExited() {
		return fmt.Errorf("%w: %s has exited", ErrNotFound, t.id)
	}
	if _, err := t.file.Write(p); err != nil {
		return fmt.Errorf("ptyterm: write to %s: %w", t.id, err)
	}
	return nil
}

// resize is authoritative, not a vote: one client renders this PTY, so the size
// it asks for is the size the process is told.
func (t *terminal) resize(cols, rows int) error {
	if t.hasExited() {
		return fmt.Errorf("%w: %s has exited", ErrNotFound, t.id)
	}
	if err := pty.Setsize(t.file, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}); err != nil { //nolint:gosec // bounded by validateSize
		return fmt.Errorf("ptyterm: resize %s: %w", t.id, err)
	}
	t.mu.Lock()
	t.cols, t.rows = cols, rows
	t.mu.Unlock()
	return nil
}

func (t *terminal) snapshot() Terminal {
	t.mu.Lock()
	defer t.mu.Unlock()
	return Terminal{ID: t.id, Title: t.title, Dir: t.dir, Command: t.command, Cols: t.cols, Rows: t.rows}
}

// close hangs the terminal up and waits for its reader to drain. Closing the
// master first raises SIGHUP on the foreground process group, which is what
// gives a shell its chance to hang up its own jobs; a kill goes first only if
// it does not take that chance.
func (t *terminal) close() {
	t.closeOnce.Do(func() {
		_ = t.file.Close()
		select {
		case <-t.readerDone:
			return
		case <-time.After(hangupGrace):
		}
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		<-t.readerDone
	})
}
