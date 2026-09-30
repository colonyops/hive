// Package ptyterm runs ephemeral terminals: this process owns a PTY and the
// process on the far end of it, with no terminal multiplexer in between.
//
// They are ephemeral in the strong sense. A terminal is this process's child,
// so it dies with the app; nothing can attach to it from outside, and closing
// it is the end of whatever was running. That is the trade a pop-up shell wants
// and the reason these are not the terminals hive sessions run in, which are
// tmux's and outlive the app (ADR ephemeral-popup-terminals).
package ptyterm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

const (
	minDimension = 1
	maxDimension = 1000

	// defaultCols/defaultRows size a terminal opened before anything measured
	// the pane. A PTY has to open at some size, and nothing else is attached
	// whose size could be inherited.
	defaultCols = 80
	defaultRows = 24

	// defaultReplayBytes is one terminal's replay ring. 256 KiB is a few screens
	// of scrollback at a normal size and a bounded cost per open terminal.
	defaultReplayBytes = 256 << 10
	// defaultBufferBytes bounds one subscriber's queue before the stream is cut.
	// Output coalesces, so this is reached only by a consumer that has stopped
	// reading rather than by one that is merely behind.
	defaultBufferBytes = 4 << 20

	// maxConcurrentSessions bounds Open across every caller. Each terminal is an
	// agent CLI spawning its own copy of every enabled MCP server, so with no
	// idle reaping and no cap this is a fork bomb with a progress bar; the
	// pop-up's former one-at-a-time behaviour was frontend policy, not a limit
	// the manager enforced (ADR ptyterm-terminals-are-caller-addressed).
	maxConcurrentSessions = 8
)

var (
	// ErrUnavailable is returned on the server build and on unsupported platforms.
	ErrUnavailable = errors.New("ptyterm: ephemeral terminals unavailable")
	// ErrInvalidSize is returned for out-of-bounds cols/rows.
	ErrInvalidSize = errors.New("ptyterm: invalid terminal size")
	// ErrInvalidSpec is returned for a spec that cannot be launched.
	ErrInvalidSpec = errors.New("ptyterm: invalid terminal spec")
	// ErrNotFound is returned for an id with no live terminal.
	ErrNotFound = errors.New("ptyterm: no such terminal")
	// ErrIDInUse reports a caller-supplied id that already addresses a live
	// terminal.
	ErrIDInUse = errors.New("ptyterm: terminal id already in use")
	// ErrInvalidID reports an id that is malformed or that collides with the
	// minted namespace.
	ErrInvalidID = errors.New("ptyterm: invalid terminal id")
	// ErrTooManyTerminals reports the concurrency cap.
	ErrTooManyTerminals = errors.New("ptyterm: too many terminals are open")
)

// idPattern is the caller-id charset: safe as a map key today and, since a
// workspace session's id is durable (ADR ptyterm-terminals-are-caller-addressed), safe wherever a future caller
// wants to fold it into a path.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// mintedIDPattern is the shape Open mints when Spec.ID is empty. A caller id
// matching it is rejected so a mint can never collide with an address a
// caller is already holding.
var mintedIDPattern = regexp.MustCompile(`^t[0-9]+$`)

// validateID accepts [A-Za-z0-9_.-]{1,64} and rejects the minted form t<N>.
func validateID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("%w: %q", ErrInvalidID, id)
	}
	if mintedIDPattern.MatchString(id) {
		return fmt.Errorf("%w: %q collides with the minted namespace", ErrInvalidID, id)
	}
	return nil
}

// Spec is one terminal's launch. Command is a shell command line rather than an
// argv: it is run through a login shell, so what a user would type into their
// own terminal — aliases, functions, a PATH set by their startup files — is
// what runs. Empty opens an interactive shell instead.
type Spec struct {
	// ID addresses the terminal. Empty mints one; a non-empty id already live
	// is ErrIDInUse rather than a second terminal, because the caller's id is
	// what it will reattach with.
	ID      string
	Dir     string
	Command string
	Cols    int
	Rows    int
}

// ManagerOptions configures the terminal set.
type ManagerOptions struct {
	// Environ answers the environment a terminal is spawned with — execenv's
	// resolved PATH in the app, whatever a test hands it otherwise. nil means
	// this process's own.
	Environ func(context.Context) []string
	// Shell overrides the shell every terminal is launched through. Empty
	// resolves $SHELL.
	Shell []string
}

// Manager owns the open terminals, keyed by an id it mints. The ids mean
// nothing outside this process's lifetime, which is also all they have to.
type Manager struct {
	environ func(context.Context) []string
	shell   []string

	mu        sync.Mutex
	terminals map[string]*terminal
	order     []string
	nextID    int
	stopped   bool
}

func NewManager(opts ManagerOptions) *Manager {
	m := &Manager{
		environ:   opts.Environ,
		shell:     opts.Shell,
		terminals: map[string]*terminal{},
	}
	if m.environ == nil {
		m.environ = func(context.Context) []string { return os.Environ() }
	}
	return m
}

// Available reports whether this build and platform can run a PTY at all. There
// is no external program to find, so unlike tmux there is nothing to discover
// and nothing a user can install to change the answer.
func (m *Manager) Available(context.Context) error {
	if !platformSupported() {
		return ErrUnavailable
	}
	return nil
}

// Open launches a terminal and returns it. Dir must already be a directory the
// process can enter — a spec is rejected rather than opening a terminal
// somewhere the caller did not ask for. A non-empty Spec.ID is honoured rather
// than minted; a mint never collides with it because validateID rejects the
// minted shape as a caller id.
func (m *Manager) Open(ctx context.Context, spec Spec) (Terminal, error) {
	if err := m.Available(ctx); err != nil {
		return Terminal{}, err
	}
	if err := validateDir(spec.Dir); err != nil {
		return Terminal{}, err
	}
	cols, rows := spec.Cols, spec.Rows
	if cols == 0 && rows == 0 {
		cols, rows = defaultCols, defaultRows
	}
	if err := validateSize(cols, rows); err != nil {
		return Terminal{}, err
	}
	if spec.ID != "" {
		if err := validateID(spec.ID); err != nil {
			return Terminal{}, err
		}
	}

	argv := m.argv(spec.Command)

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return Terminal{}, ErrUnavailable
	}
	if len(m.terminals) >= maxConcurrentSessions {
		m.mu.Unlock()
		return Terminal{}, ErrTooManyTerminals
	}
	id := spec.ID
	if id == "" {
		m.nextID++
		id = fmt.Sprintf("t%d", m.nextID)
	} else if _, exists := m.terminals[id]; exists {
		m.mu.Unlock()
		return Terminal{}, ErrIDInUse
	}
	m.mu.Unlock()

	t, err := spawn(spawnOptions{
		ID:      id,
		Title:   title(spec.Command, argv),
		Dir:     spec.Dir,
		Command: spec.Command,
		Argv:    argv,
		Env:     terminalEnv(m.environ(ctx)),
		Cols:    cols,
		Rows:    rows,
	}, m.forget)
	if err != nil {
		return Terminal{}, err
	}

	m.mu.Lock()
	switch {
	case m.stopped:
		m.mu.Unlock()
		t.close()
		return Terminal{}, ErrUnavailable
	case spec.ID != "" && m.terminals[id] != nil:
		// Another Open for the same id won the race while this one spawned.
		m.mu.Unlock()
		t.close()
		return Terminal{}, ErrIDInUse
	case len(m.terminals) >= maxConcurrentSessions:
		// The cap filled while this one spawned.
		m.mu.Unlock()
		t.close()
		return Terminal{}, ErrTooManyTerminals
	}
	m.terminals[id] = t
	m.order = append(m.order, id)
	m.mu.Unlock()
	return t.snapshot(), nil
}

// List reports the open terminals, oldest first.
func (m *Manager) List() []Terminal {
	m.mu.Lock()
	terminals := make([]*terminal, 0, len(m.order))
	for _, id := range m.order {
		if t, ok := m.terminals[id]; ok {
			terminals = append(terminals, t)
		}
	}
	m.mu.Unlock()

	out := make([]Terminal, 0, len(terminals))
	for _, t := range terminals {
		out = append(out, t.snapshot())
	}
	return out
}

// Close ends a terminal and whatever is running in it, and reports whether
// there was one to close.
func (m *Manager) Close(id string) (bool, error) {
	m.mu.Lock()
	t, ok := m.terminals[id]
	if ok {
		m.dropLocked(id)
	}
	m.mu.Unlock()
	if !ok {
		return false, nil
	}
	t.close()
	return true, nil
}

// Subscribe opens a terminal's event stream. There is one subscriber per
// terminal; a second call closes the first channel.
func (m *Manager) Subscribe(id string) (<-chan Event, func(), error) {
	t, err := m.terminal(id)
	if err != nil {
		return nil, nil, err
	}
	ch, unsubscribe := t.subscribe()
	return ch, unsubscribe, nil
}

func (m *Manager) Write(id string, p []byte) error {
	t, err := m.terminal(id)
	if err != nil {
		return err
	}
	return t.write(p)
}

// Resize sets a terminal's size. It is not a vote: one client renders this PTY,
// so the size asked for is the size the process is told.
func (m *Manager) Resize(id string, cols, rows int) error {
	if err := validateSize(cols, rows); err != nil {
		return err
	}
	t, err := m.terminal(id)
	if err != nil {
		return err
	}
	return t.resize(cols, rows)
}

// Stop closes every terminal. Idempotent.
func (m *Manager) Stop(context.Context) error {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return nil
	}
	m.stopped = true
	terminals := make([]*terminal, 0, len(m.terminals))
	for _, t := range m.terminals {
		terminals = append(terminals, t)
	}
	m.terminals = map[string]*terminal{}
	m.order = nil
	m.mu.Unlock()

	for _, t := range terminals {
		t.close()
	}
	return nil
}

func (m *Manager) terminal(id string) (*terminal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.terminals[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return t, nil
}

// forget drops a terminal whose process exited. An exited terminal is gone
// rather than kept as a dead tab: reopening one costs a millisecond, and the
// stream has already carried the exit notice to whatever was watching.
func (m *Manager) forget(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropLocked(id)
}

func (m *Manager) dropLocked(id string) {
	delete(m.terminals, id)
	for i, existing := range m.order {
		if existing == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

// argv is what actually gets exec'd. Everything runs through a login shell so
// the user's startup files do, which is also what gives a terminal the PATH
// their own terminal has (ADR subprocess-environment) — execenv's resolved environment is the
// floor under it, not a replacement for it.
func (m *Manager) argv(command string) []string {
	shell := m.shell
	if len(shell) == 0 {
		shell = []string{resolveShell(), "-l"}
	}
	if strings.TrimSpace(command) == "" {
		return shell
	}
	return append(append([]string{}, shell...), "-c", command)
}

func resolveShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	for _, candidate := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return "/bin/sh"
}

// title labels a terminal in a tab or a window chrome: the program a launcher
// asked for, or the shell itself.
func title(command string, argv []string) string {
	if fields := strings.Fields(command); len(fields) > 0 {
		return filepath.Base(fields[0])
	}
	if len(argv) > 0 {
		return filepath.Base(argv[0])
	}
	return "shell"
}

// terminalEnv declares what the emulator on the far end of the socket can do.
// xterm.js renders 256 colours and true colour, and a shell that is not told so
// degrades its prompt for a terminal that is not this one.
func terminalEnv(base []string) []string {
	out := make([]string, 0, len(base)+3)
	for _, kv := range base {
		switch {
		case strings.HasPrefix(kv, "TERM="), strings.HasPrefix(kv, "COLORTERM="), strings.HasPrefix(kv, "TERM_PROGRAM="):
			continue
		case strings.HasPrefix(kv, "TMUX="), strings.HasPrefix(kv, "TMUX_PANE="):
			// Hive may itself have been launched from inside tmux; inherited
			// client variables would tell the shell it is in a multiplexer that
			// is not on the other end of this PTY.
			continue
		}
		out = append(out, kv)
	}
	return append(out, "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=hive-desktop")
}

func validateDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("%w: no directory", ErrInvalidSpec)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: %s is not a directory", ErrInvalidSpec, dir)
	}
	return nil
}

func validateSize(cols, rows int) error {
	if cols < minDimension || cols > maxDimension || rows < minDimension || rows > maxDimension {
		return fmt.Errorf("%w: %dx%d", ErrInvalidSize, cols, rows)
	}
	return nil
}

func platformSupported() bool {
	return buildSupportsTerminal && (runtime.GOOS == "darwin" || runtime.GOOS == "linux")
}
