// Package panebridge puts the terminal it runs in onto one tmux pane of a
// hive session, through the desktop's pane stream. A native terminal surface
// runs it as its command, so libghostty draws a pane that tmux still owns:
// tmux keeps the layout, the size and the session; only the renderer moves
// out of the webview.
package panebridge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

// Flag starts the executable in pane-bridge mode:
// `<exe> --tmux-pane <session> <pane>`. The stream's URL and token come from
// the environment (EnvURL, EnvToken), so the token stays out of process
// listings.
const Flag = "--tmux-pane"

const (
	EnvURL   = "HIVE_PANE_STREAM_URL"
	EnvToken = "HIVE_PANE_STREAM_TOKEN"
)

// Command is the command line a surface runs for slug's pane.
func Command(exe, slug, pane string) string {
	return strings.Join([]string{shellQuote(exe), Flag, shellQuote(slug), shellQuote(pane)}, " ")
}

// Main runs the bridge when args select it, and reports whether they did.
func Main(ctx context.Context, args []string) (int, bool) {
	if len(args) != 4 || args[1] != Flag {
		return 0, false
	}
	return Run(ctx, os.Getenv(EnvURL), os.Getenv(EnvToken), args[2], args[3]), true
}

// Run relays stdin and stdout to the pane until the stream or stdin ends.
func Run(ctx context.Context, streamURL, token, slug, pane string) int {
	in, out := os.Stdin, os.Stdout
	fd := int(in.Fd())
	if term.IsTerminal(fd) {
		old, err := term.MakeRaw(fd)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "hive:", err)
			return 2
		}
		defer func() { _ = term.Restore(fd, old) }()
	}

	u, err := url.Parse(streamURL)
	if err != nil || streamURL == "" {
		_, _ = fmt.Fprintf(out, "\r\nhive: no pane stream to attach to\r\n")
		return 1
	}
	q := u.Query()
	q.Set("slug", slug)
	q.Set("pane", pane)
	u.RawQuery = q.Encode()

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, resp, err := websocket.Dial(dialCtx, u.String(), &websocket.DialOptions{
		HTTPHeader:      http.Header{"Authorization": {"Bearer " + token}},
		CompressionMode: websocket.CompressionDisabled,
	})
	cancel()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		_, _ = fmt.Fprintf(out, "\r\nhive: cannot attach pane %s of %s: %v\r\n", pane, slug, err)
		return 1
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(-1)

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	go relayInput(ctx, cancel, in, conn)
	go requestRepaints(ctx, conn)
	for {
		_, p, err := conn.Read(ctx)
		if err != nil {
			return 0
		}
		if _, err := out.Write(p); err != nil {
			return 0
		}
	}
}

// escapeTime is how long an escape sequence cut off at the end of a read
// waits for its rest before it is sent as is: a lone Esc is a key (tmux's
// escape-time, which hc attach uses too).
const escapeTime = 50 * time.Millisecond

func relayInput(ctx context.Context, cancel context.CancelFunc, in io.Reader, conn *websocket.Conn) {
	defer cancel()
	chunks := make(chan []byte, 16)
	go func() {
		defer close(chunks)
		buf := make([]byte, 32<<10)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				chunks <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()
	send := func(p []byte) bool {
		if typed := dropReplies(p); len(typed) > 0 {
			return conn.Write(ctx, websocket.MessageBinary, typed) == nil
		}
		return true
	}
	var carry []byte
	flush := time.NewTimer(time.Hour)
	flush.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-flush.C:
			if !send(carry) {
				return
			}
			carry = nil
		case chunk, ok := <-chunks:
			if !ok {
				return
			}
			flush.Stop()
			data := slices.Concat(carry, chunk)
			cut := incompleteTail(data)
			carry = append([]byte(nil), data[cut:]...)
			if !send(data[:cut]) {
				return
			}
			if len(carry) > 0 {
				flush.Reset(escapeTime)
			}
		}
	}
}

// resizeSettle lets a run of resizes (a divider or window drag) finish
// before one repaint follows it.
const resizeSettle = 40 * time.Millisecond

// requestRepaints asks for a fresh snapshot after the surface resizes. The
// surface only resizes once tmux has (pane boxes come from tmux's layout), and
// both reflow differently, so the program's next relative cursor moves would
// land on the wrong lines of what this surface shows.
func requestRepaints(ctx context.Context, conn *websocket.Conn) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	settle := time.NewTimer(time.Hour)
	settle.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-winch:
			settle.Reset(resizeSettle)
		case <-settle.C:
			if conn.Write(ctx, websocket.MessageText, []byte("repaint")) != nil {
				return
			}
		}
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
