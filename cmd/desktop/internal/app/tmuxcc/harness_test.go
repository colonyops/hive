//go:build !server

package tmuxcc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTmux is an in-memory control-mode server: it satisfies the process seam,
// answers commands with %begin/%end blocks, and lets a test inject arbitrary
// notification lines.
type fakeTmux struct {
	t    *testing.T
	slug string

	stdinR  *io.PipeReader
	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stdoutW *io.PipeWriter
	done    chan struct{}

	closeOnce sync.Once

	// writeMu keeps a reply block contiguous without holding mu across the
	// pipe write, which blocks whenever the client is not reading — including
	// while it tears down, where Kill needs mu.
	writeMu sync.Mutex

	mu        sync.Mutex
	num       int
	kills     int
	commands  []string
	windows   []string
	panes     map[string][]string
	captures  map[string][]string
	histories map[string][]string
	cursors   map[string]string
	failures  map[string]string
	splitPane string
	onCommand func(cmd string)
	closed    bool
}

func newFakeTmux(t *testing.T, slug string) *fakeTmux {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	f := &fakeTmux{
		t:         t,
		slug:      slug,
		stdinR:    stdinR,
		stdinW:    stdinW,
		stdoutR:   stdoutR,
		stdoutW:   stdoutW,
		done:      make(chan struct{}),
		panes:     map[string][]string{},
		captures:  map[string][]string{},
		histories: map[string][]string{},
		cursors:   map[string]string{},
		failures:  map[string]string{},
	}
	t.Cleanup(f.closeStreams)
	return f
}

func (f *fakeTmux) factory() func(Options) process {
	return func(Options) process { return f }
}

func (f *fakeTmux) Start(context.Context) (io.Writer, io.Reader, error) {
	go f.serve()
	return f.stdinW, f.stdoutR, nil
}

func (f *fakeTmux) Wait() error {
	<-f.done
	return nil
}

func (f *fakeTmux) Kill() error {
	f.mu.Lock()
	f.kills++
	f.mu.Unlock()
	f.closeStreams()
	return nil
}

// killed reports how many times the client killed the process. Teardown marks
// the client dead before it kills, so a non-zero count means the exit is
// already visible to anyone inspecting the client.
func (f *fakeTmux) killed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.kills
}

func (f *fakeTmux) closeStreams() {
	f.closeOnce.Do(func() {
		f.mu.Lock()
		f.closed = true
		f.mu.Unlock()
		_ = f.stdoutW.Close()
		_ = f.stdinR.Close()
	})
}

func (f *fakeTmux) serve() {
	defer close(f.done)

	f.write("%begin 100 0 0", "%end 100 0 0", "%session-changed $1 "+f.slug)

	scanner := bufio.NewScanner(f.stdinR)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		cmd := scanner.Text()
		f.mu.Lock()
		f.commands = append(f.commands, cmd)
		hook := f.onCommand
		f.mu.Unlock()
		if hook != nil {
			hook(cmd)
		}
		f.respond(cmd)
	}
}

func (f *fakeTmux) respond(cmd string) {
	f.mu.Lock()
	for prefix, msg := range f.failures {
		if strings.HasPrefix(cmd, prefix) {
			f.mu.Unlock()
			f.reply([]string{msg}, true)
			return
		}
	}
	f.mu.Unlock()

	switch {
	case cmd == "detach":
		f.reply(nil, false)
		f.emit("%exit")
		f.closeStreams()
	case strings.HasPrefix(cmd, "list-windows"):
		f.mu.Lock()
		windows := append([]string(nil), f.windows...)
		f.mu.Unlock()
		f.reply(windows, false)
	case strings.HasPrefix(cmd, "list-panes"):
		f.mu.Lock()
		panes := append([]string(nil), f.panes[argAfter(cmd, "-t")]...)
		f.mu.Unlock()
		f.reply(panes, false)
	case strings.HasPrefix(cmd, "capture-pane"):
		f.mu.Lock()
		// -E -1 ends the capture on the last history row, which is how a first
		// paint asks for scrollback rather than the visible screen.
		lines := f.captures[argAfter(cmd, "-t")]
		if strings.Contains(cmd, "-E -1") {
			lines = f.histories[argAfter(cmd, "-t")]
		}
		f.mu.Unlock()
		f.reply(lines, false)
	case strings.HasPrefix(cmd, "display-message"):
		f.mu.Lock()
		reported, ok := f.cursors[argAfter(cmd, "-t")]
		f.mu.Unlock()
		if !ok {
			f.reply(nil, false)
			return
		}
		f.reply([]string{reported}, false)
	case strings.HasPrefix(cmd, "new-window"):
		f.reply([]string{"@9"}, false)
	case strings.HasPrefix(cmd, "split-window"):
		f.mu.Lock()
		pane := f.splitPane
		f.mu.Unlock()
		f.reply([]string{pane}, false)
	default:
		f.reply(nil, false)
	}
}

func (f *fakeTmux) reply(lines []string, failed bool) {
	f.mu.Lock()
	f.num++
	ts, num := 1000+f.num, f.num
	f.mu.Unlock()

	guard := "%end"
	if failed {
		guard = "%error"
	}
	block := make([]string, 0, len(lines)+2)
	block = append(block, fmt.Sprintf("%%begin %d %d 1", ts, num))
	block = append(block, lines...)
	block = append(block, fmt.Sprintf("%s %d %d 1", guard, ts, num))
	f.write(block...)
}

func (f *fakeTmux) emit(line string) { f.write(line) }

// write puts whole lines on stdout without holding mu: an io.Pipe write parks
// until the client reads, and a client that is tearing down never will.
func (f *fakeTmux) write(lines ...string) {
	f.writeMu.Lock()
	defer f.writeMu.Unlock()

	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if closed {
		return
	}
	for _, line := range lines {
		if _, err := io.WriteString(f.stdoutW, line+"\n"); err != nil {
			f.mu.Lock()
			f.closed = true
			f.mu.Unlock()
			return
		}
	}
}

func (f *fakeTmux) setWindows(lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.windows = lines
}

// setPanes sets what list-panes answers for one window target.
func (f *fakeTmux) setPanes(window string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.panes[window] = lines
}

func (f *fakeTmux) setOnCommand(hook func(cmd string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onCommand = hook
}

// setCapture sets a pane's visible screen. It is the only capture most tests
// need; setHistory and setCursor cover the rest of a first paint.
func (f *fakeTmux) setCapture(pane string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.captures[pane] = lines
}

func (f *fakeTmux) setHistory(pane string, lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.histories[pane] = lines
}

func (f *fakeTmux) setCursor(pane string, row, col int) {
	f.setPaneState(pane, row, col, 0, false)
}

func (f *fakeTmux) setPaneState(pane string, row, col, mouseProtocol int, sgr bool) {
	standard, button, all := 0, 0, 0
	switch mouseProtocol {
	case 1000:
		standard = 1
	case 1002:
		button = 1
	case 1003:
		all = 1
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cursors[pane] = fmt.Sprintf("%d %d %d %d %d %d", row, col, standard, button, all, boolInt(sgr))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (f *fakeTmux) sentCommands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

func (f *fakeTmux) commandsMatching(substr string) []string {
	var matched []string
	for _, cmd := range f.sentCommands() {
		if strings.Contains(cmd, substr) {
			matched = append(matched, cmd)
		}
	}
	return matched
}

func (f *fakeTmux) setSplitPane(pane string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.splitPane = pane
}

func (f *fakeTmux) countCommands(prefix string) int {
	n := 0
	for _, cmd := range f.sentCommands() {
		if strings.HasPrefix(cmd, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeTmux) awaitCommands(t *testing.T, prefix string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f.countCommands(prefix) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d %q commands, saw %v", want, prefix, f.sentCommands())
}

func argAfter(cmd, flag string) string {
	fields := strings.Fields(cmd)
	for i, field := range fields {
		if field == flag && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// attachFake wires a client to a fake server with one window unless the test
// configured otherwise.
func attachFake(t *testing.T, f *fakeTmux, opts Options) *Client {
	t.Helper()
	if len(f.windows) == 0 {
		f.setWindows("@1 1 %1 120 40 0 b25f,120x40,0,0,1 claude")
	}
	opts.Slug = f.slug
	if opts.Cols == 0 {
		opts.Cols, opts.Rows = 80, 24
	}
	opts.newProcess = f.factory()

	ctx := t.Context()
	client, err := Attach(ctx, ctx, opts)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	t.Cleanup(func() { _ = client.Close(context.WithoutCancel(ctx)) })
	return client
}

// collect reads events until stop returns true, and fails the test if that
// never happens.
func collect(t *testing.T, ch <-chan Event, stop func(Event) bool) []Event {
	t.Helper()
	var events []Event
	timeout := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("event channel closed after %d events", len(events))
			}
			events = append(events, ev)
			if stop(ev) {
				return events
			}
		case <-timeout:
			t.Fatalf("timed out after %d events: %#v", len(events), events)
		}
	}
}
