package tmuxcc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func testLogger() zerolog.Logger { return zerolog.Nop() }

// sendResult carries a Send's outcome off the goroutine that made it, so
// assertions stay on the test goroutine.
type sendResult struct {
	cmd   string
	lines []string
	err   error
}

// recordingWriter is the control client's stdin: it captures command order so
// a test can answer in the order tmux would.
type recordingWriter struct {
	mu    sync.Mutex
	lines []string
	err   error
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	w.lines = append(w.lines, strings.TrimSuffix(string(p), "\n"))
	return len(p), nil
}

func (w *recordingWriter) commands() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

func (w *recordingWriter) await(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cmds := w.commands(); len(cmds) >= n {
			return cmds
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d commands, saw %v", n, w.commands())
	return nil
}

func newTestGateway(notify func(Notification)) (*Gateway, *recordingWriter) {
	w := &recordingWriter{}
	if notify == nil {
		notify = func(Notification) {}
	}
	return NewGateway(w, notify, testLogger()), w
}

func sendAsync(ctx context.Context, g *Gateway, cmd string) <-chan sendResult {
	out := make(chan sendResult, 1)
	go func() {
		lines, err := g.Send(ctx, cmd)
		out <- sendResult{cmd: cmd, lines: lines, err: err}
	}()
	return out
}

func feedAll(t *testing.T, g *Gateway, lines ...string) {
	t.Helper()
	for _, line := range lines {
		require.NoError(t, g.Feed([]byte(line)))
	}
}

// A server-originated block (flags 0) is the attach preamble. Consuming the
// FIFO for it would misattribute every later reply.
func TestFlagsZeroBlockDoesNotDequeue(t *testing.T) {
	t.Parallel()

	g, w := newTestGateway(nil)
	done := sendAsync(t.Context(), g, "list-windows")
	w.await(t, 1)

	feedAll(t, g, "%begin 100 0 0", "server chatter", "%end 100 0 0")
	select {
	case got := <-done:
		t.Fatalf("command resolved from a server block: %#v", got)
	case <-time.After(50 * time.Millisecond):
	}

	feedAll(t, g, "%begin 200 1 1", "@1 1 %1 claude", "%end 200 1 1")
	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, []string{"@1 1 %1 claude"}, got.lines)
}

func TestGuardDesyncIsFatal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		lines []string
	}{
		{"end without begin", []string{"%end 100 1 1"}},
		{"error without begin", []string{"%error 100 1 1"}},
		{"malformed begin", []string{"%begin nope 1 1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, w := newTestGateway(nil)
			pending := sendAsync(context.WithoutCancel(t.Context()), g, "list-windows")
			w.await(t, 1)

			var err error
			for _, line := range tc.lines {
				if err = g.Feed([]byte(line)); err != nil {
					break
				}
			}

			var pe *protocolError
			require.ErrorAs(t, err, &pe)
			require.ErrorAs(t, (<-pending).err, &pe, "in-flight command aborts")

			later := <-sendAsync(t.Context(), g, "later")
			require.ErrorAs(t, later.err, &pe, "the gateway stays closed")
		})
	}
}

func TestBeginWithNoCommandOutstandingIsFatal(t *testing.T) {
	t.Parallel()

	g, _ := newTestGateway(nil)

	var pe *protocolError
	require.ErrorAs(t, g.Feed([]byte("%begin 100 1 1")), &pe)
}

// capture-pane hands us the pane's screen verbatim, so a line of terminal
// output can be shaped exactly like a guard. Inside an open block only the
// guard that closes it counts; everything else is content, or a session dies
// over a screenful of text.
func TestGuardShapedLinesInsideABlockAreContent(t *testing.T) {
	t.Parallel()

	g, w := newTestGateway(nil)
	done := sendAsync(t.Context(), g, "capture-pane -pe -J -t %1")
	w.await(t, 1)

	screen := []string{"%end 100 0 1", "%begin 100 0 1", "%error 12 34"}
	feedAll(t, g, append(append([]string{"%begin 1000 1 1"}, screen...), "%end 1000 1 1")...)

	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, screen, got.lines)

	next := sendAsync(t.Context(), g, "list-windows")
	w.await(t, 2)
	feedAll(t, g, "%begin 1001 2 1", "@1 1 %1 claude", "%end 1001 2 1")
	later := <-next
	require.NoError(t, later.err, "the block still paired with its own command")
	require.Equal(t, []string{"@1 1 %1 claude"}, later.lines)
}

// A canceled Send still owns its FIFO slot: tmux will answer it, and that
// answer must not be handed to the next caller.
func TestCanceledSendLeavesTombstone(t *testing.T) {
	t.Parallel()

	g, w := newTestGateway(nil)

	canceled, cancel := context.WithCancel(t.Context())
	first := sendAsync(canceled, g, "slow-command")
	w.await(t, 1)
	cancel()
	require.ErrorIs(t, (<-first).err, context.Canceled)

	second := sendAsync(t.Context(), g, "second-command")
	w.await(t, 2)

	feedAll(t, g, "%begin 100 1 1", "reply to slow", "%end 100 1 1")
	feedAll(t, g, "%begin 200 2 1", "reply to second", "%end 200 2 1")

	got := <-second
	require.NoError(t, got.err)
	require.Equal(t, []string{"reply to second"}, got.lines)
}

func TestConcurrentSendsStayPaired(t *testing.T) {
	t.Parallel()

	g, w := newTestGateway(nil)

	const n = 8
	pending := make([]<-chan sendResult, 0, n)
	for i := range n {
		pending = append(pending, sendAsync(t.Context(), g, fmt.Sprintf("cmd-%d", i)))
	}

	for i, cmd := range w.await(t, n) {
		feedAll(t, g,
			fmt.Sprintf("%%begin %d %d 1", 100+i, i),
			"reply to "+cmd,
			fmt.Sprintf("%%end %d %d 1", 100+i, i),
		)
	}

	for _, ch := range pending {
		got := <-ch
		require.NoError(t, got.err)
		require.Equal(t, []string{"reply to " + got.cmd}, got.lines)
	}
}

func TestErrorReplyIsCommandError(t *testing.T) {
	t.Parallel()

	g, w := newTestGateway(nil)
	done := sendAsync(t.Context(), g, "kill-window -t @99")
	w.await(t, 1)

	feedAll(t, g, "%begin 100 1 1", "can't find window: @99", "%error 100 1 1")

	var cmdErr *CommandError
	require.ErrorAs(t, (<-done).err, &cmdErr)
	require.Equal(t, "kill-window -t @99", cmdErr.Command)
	require.Equal(t, "can't find window: @99", cmdErr.Message)
}

func TestMalformedNonGuardLinesAreDropped(t *testing.T) {
	t.Parallel()

	var seen []Notification
	g, _ := newTestGateway(func(n Notification) { seen = append(seen, n) })

	feedAll(t, g,
		"%output",                        // no pane id
		"%output @1 data",                // window id where a pane belongs
		"%window-add banana",             // not a window id
		"%window-pane-changed @1",        // missing pane
		"%session-changed 1 name",        // missing sigil
		"%totally-unknown some args",     // notification this client does not model
		"not a notification at all",      // stray garbage
		"%extended-output %1 12 nocolon", // no payload separator
	)
	require.Empty(t, seen)

	feedAll(t, g, "%window-add @7")
	require.Equal(t, []Notification{WindowAddNotification{Window: "@7"}}, seen, "the stream stays usable")
}

func TestFeedRoutesNotificationsInOrder(t *testing.T) {
	t.Parallel()

	var seen []Notification
	g, _ := newTestGateway(func(n Notification) { seen = append(seen, n) })

	feedAll(t, g,
		"%session-changed $1 hive",
		`%output %1 hello\015\012`,
		"%window-renamed @1 shell",
		"%exit server exited",
	)

	require.Equal(t, []Notification{
		SessionChanged{Session: "$1", Name: "hive"},
		OutputNotification{Pane: "%1", Data: []byte("hello\r\n")},
		WindowRenamedNotification{Window: "@1", Name: "shell"},
		ExitNotification{Reason: "server exited"},
	}, seen)
}

// bufio.Scanner would truncate this line at 64KB and desync the stream.
func TestOversizedOutputLineIsFramedWhole(t *testing.T) {
	t.Parallel()

	payload := strings.Repeat("x", 70_000)
	scanner := newLineScanner(strings.NewReader("%output %1 "+payload+"\n%window-add @2\n"), maxLineBytes)

	var seen []Notification
	g, _ := newTestGateway(func(n Notification) { seen = append(seen, n) })
	for {
		line, err := scanner.next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		require.NoError(t, g.Feed(line))
	}

	require.Len(t, seen, 2)
	out, ok := seen[0].(OutputNotification)
	require.True(t, ok)
	require.Len(t, out.Data, len(payload))
}

func TestLineScannerRefusesUnframeableInput(t *testing.T) {
	t.Parallel()

	scanner := newLineScanner(strings.NewReader(strings.Repeat("x", 200_000)), 128)
	_, err := scanner.next()
	require.ErrorIs(t, err, errLineTooLong)
}

func TestSendFailsWhenStdinIsBroken(t *testing.T) {
	t.Parallel()

	g := NewGateway(&recordingWriter{err: errors.New("broken pipe")}, func(Notification) {}, testLogger())

	_, err := g.Send(t.Context(), "list-windows")
	require.Error(t, err)

	_, err = g.Send(t.Context(), "list-windows")
	require.Error(t, err, "a write failure closes the gateway")
}

// capture-pane -e replies carry raw escape sequences; they must survive the
// gateway byte for byte.
func TestGatewayCarriesReplyBytesVerbatim(t *testing.T) {
	t.Parallel()

	g, w := newTestGateway(nil)
	done := sendAsync(t.Context(), g, "capture-pane -pe -J -t %1")
	w.await(t, 1)

	screen := "\x1b[1mbold\x1b[0m"
	feedAll(t, g, "%begin 100 1 1", screen, "%end 100 1 1")

	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, []string{screen}, got.lines)
}

func TestTrimEOLStripsCarriageReturn(t *testing.T) {
	t.Parallel()

	require.Equal(t, []byte("%noop"), trimEOL([]byte("%noop\r\n")))
	require.Equal(t, []byte("%noop"), trimEOL([]byte("%noop\n")))
	require.Equal(t, []byte("%noop"), trimEOL([]byte("%noop")))
}

func TestGatewayCapturesTheAttachPreambleError(t *testing.T) {
	t.Parallel()
	gw := NewGateway(io.Discard, func(Notification) {}, testLogger())

	require.NoError(t, gw.Feed([]byte("%begin 100 0 0")))
	require.NoError(t, gw.Feed([]byte("no server running on /private/tmp/tmux-501/default")))
	require.NoError(t, gw.Feed([]byte("%error 100 0 0")))

	require.Equal(t, "no server running on /private/tmp/tmux-501/default", gw.ServerError())
}

func TestSocketFromTMUX(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/private/tmp/tmux-501/default", socketFromTMUX("/private/tmp/tmux-501/default,24757,4"))
	require.Empty(t, socketFromTMUX(""))
	require.Empty(t, socketFromTMUX("no-commas-here"))
}
