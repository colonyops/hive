package tmuxcc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// maxLineBytes caps one control-mode line. A single %output line routinely
// exceeds bufio.Scanner's 64KB default, so the framer grows instead of
// truncating; past this bound the stream is treated as unframeable.
const maxLineBytes = 8 << 20

// CommandError is a tmux %error reply. The app service classifies it into an
// error Kind; nothing matches on its text inside this package.
type CommandError struct {
	Command string
	Message string
}

func (e *CommandError) Error() string {
	if e.Message == "" {
		return "tmuxcc: command failed: " + e.Command
	}
	return "tmuxcc: command failed: " + e.Command + ": " + e.Message
}

// protocolError is an unrecoverable framing desync — a guard line outside any
// block that cannot be paired. Such a line is never dropped-and-logged: once
// the begin/end pairing is lost, every later reply is attributed to the wrong
// command, so the client tears down instead.
type protocolError struct{ msg string }

func (e *protocolError) Error() string { return "tmuxcc: protocol desync: " + e.msg }

var (
	errGatewayClosed = errors.New("tmuxcc: control connection closed")
	errLineTooLong   = errors.New("tmuxcc: control line exceeds limit")
)

type reply struct {
	lines []string
	err   error
}

type pendingCommand struct {
	cmd      string
	done     chan reply
	canceled bool
}

type block struct {
	ts      string
	num     string
	discard bool
	cmd     *pendingCommand
	lines   []string
}

// Gateway is the protocol engine: it serializes command writes to the
// control-mode client's stdin, pairs each %begin/%end block with the command
// that produced it, and routes interleaved notifications. It owns no transport
// beyond the io.Writer it was handed.
type Gateway struct {
	w      io.Writer
	notify func(Notification)
	log    zerolog.Logger

	writeMu sync.Mutex

	mu        sync.Mutex
	queue     []*pendingCommand
	open      *block
	closed    bool
	closeErr  error
	serverErr string
}

// NewGateway wires a gateway over the control client's stdin. notify runs on
// the reader goroutine in stream order and MUST NOT block: a pending command's
// %end reply shares that goroutine.
func NewGateway(stdin io.Writer, notify func(Notification), log zerolog.Logger) *Gateway {
	return &Gateway{w: stdin, notify: notify, log: log}
}

// Feed advances the state machine by one control-mode line (CR already
// stripped). line must not be retained. It returns a non-nil error only for a
// fatal desync, which also fails every pending command.
//
// Inside an open block only the guard that closes it is a guard: capture-pane
// replies carry screen contents verbatim, and a line of pane output can look
// like any of them. Anything else — a mismatched %end, a well-formed %begin —
// is reply content (iTerm2 reads the protocol the same way). A block that never
// closes therefore surfaces as the caller's Send timing out rather than as a
// desync, which is bounded and does not kill a session over a screenful of
// text.
func (g *Gateway) Feed(line []byte) error {
	kind, ts, num, flags, wellFormed := classifyGuard(line)

	g.mu.Lock()
	switch {
	case g.open != nil:
		closes := (kind == guardEnd || kind == guardError) && wellFormed && ts == g.open.ts && num == g.open.num
		if !closes {
			g.open.lines = append(g.open.lines, string(line))
			g.mu.Unlock()
			return nil
		}
		b := g.open
		g.open = nil
		g.resolveLocked(b, kind == guardError)
		g.mu.Unlock()
		return nil

	case kind == guardBegin:
		if !wellFormed {
			g.mu.Unlock()
			return g.abort("malformed %begin")
		}
		// flags&1 marks a reply to a command we sent. Server-originated blocks
		// (the attach preamble) must not consume the FIFO.
		if flags&1 == 0 {
			g.open = &block{ts: ts, num: num, discard: true}
			g.mu.Unlock()
			return nil
		}
		if len(g.queue) == 0 {
			g.mu.Unlock()
			return g.abort("%begin with empty command queue")
		}
		g.open = &block{ts: ts, num: num, cmd: g.queue[0]}
		g.queue = g.queue[1:]
		g.mu.Unlock()
		return nil

	case kind != guardNone:
		g.mu.Unlock()
		return g.abort("%end without %begin")

	default:
		g.mu.Unlock()
		g.dispatch(line)
		return nil
	}
}

// ServerError reports the content of the last server-originated %error block,
// if any — the text tmux prints when it rejects the attach outright.
func (g *Gateway) ServerError() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.serverErr
}

// Send writes cmd and blocks until its %end (reply lines) or %error
// (*CommandError). A ctx cancellation leaves a tombstone in the FIFO so the
// reply tmux still sends is drained rather than handed to the next caller.
func (g *Gateway) Send(ctx context.Context, cmd string) ([]string, error) {
	p := &pendingCommand{cmd: cmd, done: make(chan reply, 1)}

	g.writeMu.Lock()
	g.mu.Lock()
	if g.closed {
		err := g.closeErr
		g.mu.Unlock()
		g.writeMu.Unlock()
		return nil, err
	}
	g.queue = append(g.queue, p)
	g.mu.Unlock()
	_, err := io.WriteString(g.w, cmd+"\n")
	g.writeMu.Unlock()

	if err != nil {
		// The command never reached tmux, so no reply will arrive to consume
		// its FIFO slot — the stream can only be resynced by tearing down.
		g.fail(err)
		return nil, err
	}

	select {
	case r := <-p.done:
		return r.lines, r.err
	case <-ctx.Done():
		g.mu.Lock()
		p.canceled = true
		g.mu.Unlock()
		return nil, ctx.Err()
	}
}

// fail aborts every in-flight and future command. Calling it more than once is
// harmless; the first error is the one callers see.
func (g *Gateway) fail(err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return
	}
	g.closed = true
	g.closeErr = err
	if g.open != nil {
		g.failPending(g.open.cmd, err)
		g.open = nil
	}
	for _, p := range g.queue {
		g.failPending(p, err)
	}
	g.queue = nil
}

func (g *Gateway) failPending(p *pendingCommand, err error) {
	if p == nil || p.canceled {
		return
	}
	p.done <- reply{err: err}
}

func (g *Gateway) abort(msg string) error {
	err := &protocolError{msg: msg}
	g.fail(err)
	return err
}

func (g *Gateway) resolveLocked(b *block, failed bool) {
	// A server-originated block that fails is tmux refusing the attach itself
	// ("no server running", "can't find session"): the process exits right
	// after, with nothing on stderr, so this text is the only diagnosis.
	if b.discard && failed && len(b.lines) > 0 {
		g.serverErr = strings.Join(b.lines, "; ")
	}
	if b.cmd == nil || b.cmd.canceled {
		return
	}
	if failed {
		b.cmd.done <- reply{err: &CommandError{Command: b.cmd.cmd, Message: strings.Join(b.lines, "\n")}}
		return
	}
	b.cmd.done <- reply{lines: b.lines}
}

func (g *Gateway) dispatch(line []byte) {
	n, err := parseNotification(line)
	if err != nil {
		name, _, _ := bytes.Cut(line, []byte(" "))
		// Only the notification name is logged: payloads carry terminal
		// contents.
		g.log.Debug().Err(err).Str("notification", string(name)).Msg("dropped control line")
		return
	}
	g.notify(n)
}

type guardKind int

const (
	guardNone guardKind = iota
	guardBegin
	guardEnd
	guardError
)

// classifyGuard parses `%begin|%end|%error <ts> <num> [flags]`. wellFormed is
// false when the line names a guard but its tuple does not parse.
func classifyGuard(line []byte) (kind guardKind, ts, num string, flags int, wellFormed bool) {
	switch {
	case bytes.HasPrefix(line, []byte("%begin ")):
		kind = guardBegin
	case bytes.HasPrefix(line, []byte("%end ")):
		kind = guardEnd
	case bytes.HasPrefix(line, []byte("%error ")):
		kind = guardError
	default:
		return guardNone, "", "", 0, false
	}

	fields := bytes.Fields(line)
	if len(fields) < 3 || !isDigits(fields[1]) || !isDigits(fields[2]) {
		return kind, "", "", 0, false
	}
	flags = 1
	if len(fields) >= 4 {
		parsed, err := strconv.Atoi(string(fields[3]))
		if err != nil {
			return kind, "", "", 0, false
		}
		flags = parsed
	}
	return kind, string(fields[1]), string(fields[2]), flags, true
}

// lineScanner frames the control stream on \n with no fixed line ceiling
// below maxLineBytes. The slice it returns aliases an internal buffer and is
// valid only until the next call.
type lineScanner struct {
	br  *bufio.Reader
	acc []byte
	max int
}

func newLineScanner(r io.Reader, max int) *lineScanner {
	return &lineScanner{br: bufio.NewReaderSize(r, 64<<10), max: max}
}

func (s *lineScanner) next() ([]byte, error) {
	s.acc = s.acc[:0]
	for {
		chunk, err := s.br.ReadSlice('\n')
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			if len(s.acc)+len(chunk) > s.max {
				return nil, errLineTooLong
			}
			s.acc = append(s.acc, chunk...)
		case err != nil:
			return nil, err
		case len(s.acc) == 0:
			return trimEOL(chunk), nil
		default:
			s.acc = append(s.acc, chunk...)
			return trimEOL(s.acc), nil
		}
	}
}

func trimEOL(line []byte) []byte {
	line = bytes.TrimSuffix(line, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r"))
}
