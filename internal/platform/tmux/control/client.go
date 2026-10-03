// Package tmuxcc is a tmux control-mode (`tmux -C`) client: it attaches to one
// existing tmux session, models that session's windows, streams each window's
// decoded output, and drives tmux with ordinary commands. It owns no transport
// and no UI — an adapter subscribes to a client's event stream and renders it.
package tmuxcc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/platform/observe"
)

const (
	minDimension = 1
	maxDimension = 1000

	attachTimeout = 10 * time.Second
	detachTimeout = 2 * time.Second

	// sendKeysChunk keeps one send-keys command far inside tmux's command
	// length limit once each byte is expanded to hex.
	sendKeysChunk = 64

	// The window name goes last: it is the only field that can contain spaces.
	listWindowsFormat = "#{window_id} #{window_active} #{pane_id} #{window_width} #{window_height} #{window_zoomed_flag} #{window_layout} #{window_name}"
	// The same row prefixed with the session it belongs to, for the one call
	// that lists the whole server. The name goes first because window_name is
	// last and may contain spaces, so only the first field can be split off.
	sessionWindowsFormat = "#{session_name} " + listWindowsFormat

	// A pane row for a caller asking what a window is running. The command goes
	// last: it is a process name, which is the only field here that can contain
	// spaces.
	listPanesFormat = "#{pane_id} #{pane_pid} #{pane_active} #{pane_dead} #{pane_current_command}"

	// The pane state a snapshot cannot recover from captured cells: cursor
	// position, mouse protocol, and mouse encoding.
	paneStateFormat = "#{cursor_y} #{cursor_x} #{mouse_standard_flag} #{mouse_button_flag} #{mouse_all_flag} #{mouse_sgr_flag}"

	// Where a target's active pane is. tmux expands it against the target of the
	// command it is given to, so it reads the pane on screen from an attached
	// client and the session's current one from a one-shot.
	currentPathFormat = "#{pane_current_path}"

	// historyLines bounds the scrollback a first paint replays. It is tmux's own
	// default history-limit, so on an unconfigured tmux it is the whole history
	// rather than a bound anyone runs into, and it costs a few hundred KB per
	// window against an 8 MiB broker.
	historyLines = 2000

	// overflowReason is the exit reason a stream that could not be resynced ends
	// with — unchanged from when crossing the bound was fatal outright, and so
	// is what the frontend does with it.
	overflowReason = "overflow"
)

var (
	// ErrUnavailable is returned on the server build, on unsupported platforms,
	// and when tmux is missing or older than 3.2.
	ErrUnavailable = errors.New("tmuxcc: control mode unavailable")
	// ErrInvalidSize is returned for out-of-bounds cols/rows.
	ErrInvalidSize = errors.New("tmuxcc: invalid terminal size")
	// ErrNotAttached is returned for a slug with no live client.
	ErrNotAttached = errors.New("tmuxcc: no client attached")
	// ErrUnknownWindow is returned for a window id absent from the window set.
	ErrUnknownWindow = errors.New("tmuxcc: unknown window")
	// ErrUnknownPane is returned for a pane id no tracked window owns.
	ErrUnknownPane = errors.New("tmuxcc: unknown pane")
	// ErrInvalidDirection is returned for a split or pane-selection direction
	// outside the vocabulary.
	ErrInvalidDirection = errors.New("tmuxcc: invalid direction")
	// ErrInvalidName is returned for a window name tmux could not carry.
	ErrInvalidName = errors.New("tmuxcc: invalid window name")
	// ErrInvalidPosition is returned for a move to a position outside the
	// session's window order.
	ErrInvalidPosition = errors.New("tmuxcc: invalid window position")
)

// Options configures one attach.
type Options struct {
	Slug string // tmux session name == Hive session slug
	// Cols and Rows are the opening size vote, and 0x0 means the caller has
	// nothing measured to vote with — see unsized.
	Cols        int
	Rows        int
	Binary      string // tmux executable; empty resolves "tmux" through $PATH
	BufferBytes int    // broker bound; 0 == defaultBufferBytes
	Logger      zerolog.Logger
	OnExit      func(slug, reason string)
	// Environ is what tmux is spawned with, minus the client variables
	// withoutTmuxClient drops — execenv's resolved environment in the app, nil
	// meaning this process's own.
	Environ []string

	// onEmit is a test seam for parking inside the attach sequence's synchronous
	// first paint, the only way to hold the attach goroutine between a client
	// finishing its paint and the manager registering it. Nil in production.
	onEmit func()

	newProcess func(Options) process
	// loadBuffer fills a named tmux buffer from a reader. It is a one-shot
	// command rather than a control-stream one because the control protocol is
	// line-oriented and pasted text is not — see Client.Paste.
	loadBuffer func(ctx context.Context, name string, content io.Reader) error
}

// unsized reports an attach that sets no client size at all. tmux ignores a
// control client until it has set one, so an unsized attach leaves the session
// at the size its other clients gave it instead of squeezing it to a
// placeholder; the first Resize is what joins the negotiation.
func (o Options) unsized() bool { return o.Cols == 0 && o.Rows == 0 }

func (o *Options) normalize() error {
	if o.Slug == "" {
		return fmt.Errorf("%w: empty session slug", ErrNotAttached)
	}
	if err := validateAttachSize(o.Cols, o.Rows); err != nil {
		return err
	}
	if o.Binary == "" {
		o.Binary = defaultBinary
	}
	if o.newProcess == nil {
		o.newProcess = newExecProcess
	}
	if o.loadBuffer == nil {
		binary, environ := o.Binary, o.Environ
		o.loadBuffer = func(ctx context.Context, name string, content io.Reader) error {
			return inputTmux(ctx, binary, environ, content, "load-buffer", "-b", name, "-")
		}
	}
	return nil
}

// Client is one control-mode connection to one Hive session.
type Client struct {
	slug   string
	log    zerolog.Logger
	onExit func(slug, reason string)
	onEmit func()

	proc       process
	gw         *Gateway
	ctrl       *controller
	events     *broker
	paint      *paintGate
	loadBuffer func(ctx context.Context, name string, content io.Reader) error

	cancel context.CancelFunc
	// lifeCtx bounds work that outlives the request that started it — the
	// deferred first paint, whether an attach or a repaint queued it. Held
	// rather than passed because the request context is cancelled the moment
	// Attach answers, which is precisely what deferring the paint means.
	lifeCtx context.Context //nolint:containedctx // see above; teardown cancels it before joining the pass

	readerDone chan struct{}
	workerDone chan struct{}
	// bgPaint tracks the deferred first paint. A WaitGroup rather than a
	// channel because an attach that fails before negotiate never starts one,
	// and teardown must not block waiting for a goroutine that does not exist.
	bgPaint sync.WaitGroup
	// repaintMu serialises the two callers that re-run first paint: a re-attach
	// and an overflow resync. They reach the client from different goroutines —
	// one under the manager's attach lock, one off the broker's callback — and
	// interleaved they would put two snapshots of the same pane on the stream in
	// an order neither chose, and pair one pass's bgPaint.Go with the other's
	// Wait.
	repaintMu     sync.Mutex
	reconcileReq  chan struct{}
	handshake     chan struct{}
	attached      chan struct{}
	handshakeOnce sync.Once

	// pasteMu makes a paste's load-and-paste one operation: interleaved with
	// another paste into the same pane, the buffer both share would land twice.
	pasteMu sync.Mutex

	mu         sync.Mutex
	exitReason string
	tornDown   bool
	relinking  map[string]int
	resyncing  bool

	closeOnce    sync.Once
	teardownOnce sync.Once
}

// Attach spawns `tmux -C attach -t <slug>` over the process seam and runs the
// attach sequence: handshake, client size, window enumeration, then a
// capture-pane first paint per pane. Live output produced during that
// sequence is buffered and replayed behind each pane's snapshot.
//
// ctx bounds the handshake and the attach commands only; lifetime bounds the
// client's own goroutines, which outlive the request that opened them.
func Attach(ctx, lifetime context.Context, opts Options) (*Client, error) {
	if err := opts.normalize(); err != nil {
		return nil, err
	}
	if !platformSupported() {
		return nil, ErrUnavailable
	}

	c := &Client{
		slug:         opts.Slug,
		log:          opts.Logger.With().Str("session", opts.Slug).Logger(),
		onExit:       opts.OnExit,
		onEmit:       opts.onEmit,
		ctrl:         newController(),
		loadBuffer:   opts.loadBuffer,
		readerDone:   make(chan struct{}),
		workerDone:   make(chan struct{}),
		reconcileReq: make(chan struct{}, 1),
		handshake:    make(chan struct{}),
		attached:     make(chan struct{}),
		relinking:    map[string]int{},
	}
	c.events = newBroker(backlogBounds{bytes: opts.BufferBytes}, c.resyncOverflow)
	c.paint = newPaintGate(c.emitOutput)

	c.proc = opts.newProcess(opts)
	stdin, stdout, err := c.proc.Start(lifetime)
	if err != nil {
		return nil, err
	}
	c.gw = NewGateway(stdin, c.onNotification, c.log)

	lifeCtx, cancel := context.WithCancel(lifetime)
	c.cancel = cancel
	c.lifeCtx = lifeCtx

	go c.read(stdout)
	go c.worker(lifeCtx)

	attachCtx, cancelAttach := context.WithTimeout(ctx, attachTimeout)
	defer cancelAttach()

	if err := c.awaitHandshake(attachCtx); err != nil {
		c.teardown("attach failed")
		return nil, err
	}
	if err := c.negotiate(attachCtx, opts); err != nil {
		c.teardown("attach failed")
		return nil, err
	}

	close(c.attached)
	c.publish(LifecycleChanged{Kind: LifecycleAttached})
	return c, nil
}

// Windows returns a snapshot of the current window set.
func (c *Client) Windows() []Window { return c.ctrl.Windows() }

// Subscribe returns this client's event channel plus its unsubscribe func.
// There is one active subscriber: a second call closes the first channel.
func (c *Client) Subscribe() (<-chan Event, func()) { return c.events.subscribe() }

// Write sends bytes to a pane. Every byte goes as hex (send-keys -H), which
// sidesteps tmux's key-name and literal parsing entirely. The pane is named
// rather than resolved from its window's active pane: a click that moves focus
// has its select-pane in flight while the first keystrokes follow it, and they
// must land where the user is typing.
func (c *Client) Write(ctx context.Context, pane string, p []byte) error {
	if _, err := c.pane(pane); err != nil {
		return err
	}
	for len(p) > 0 {
		n := min(sendKeysChunk, len(p))
		var cmd strings.Builder
		cmd.WriteString("send-keys -H -t ")
		cmd.WriteString(pane)
		for _, b := range p[:n] {
			fmt.Fprintf(&cmd, " %02x", b)
		}
		if _, err := c.gw.Send(ctx, cmd.String()); err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}

// Paste inserts text into a pane as a paste rather than as keystrokes. tmux
// applies the brackets because tmux is the only side that
// knows whether the pane's program asked for them: a first paint reconstructs
// mouse input state, but not bracketed-paste mode, and tmux never re-sends that
// mode to a control client (ADR pastes-are-tmux-paste-buffer-operations-not-keystrokes).
//
// The text rides tmux's stdin rather than an argument so it stays out of the
// process table, and the buffer is named so it stays out of the numbered stack
// holding the user's own copies.
func (c *Client) Paste(ctx context.Context, pane string, p []byte) error {
	if _, err := c.pane(pane); err != nil {
		return err
	}
	if len(p) == 0 {
		return nil
	}
	buffer := "hive-paste-" + strings.TrimPrefix(pane, "%")

	c.pasteMu.Lock()
	defer c.pasteMu.Unlock()
	if err := c.loadBuffer(ctx, buffer, bytes.NewReader(p)); err != nil {
		return err
	}
	_, err := c.gw.Send(ctx, "paste-buffer -d -p -b "+buffer+" -t "+pane)
	return err
}

// Resize renegotiates the client size. Every client attached to a window
// renders the same grid and tmux's window-size option picks whose size that is
// — by default the most recently used client's — so this is a vote, and
// %layout-change is the answer.
func (c *Client) Resize(ctx context.Context, cols, rows int) error {
	if err := validateSize(cols, rows); err != nil {
		return err
	}
	_, err := c.gw.Send(ctx, fmt.Sprintf("refresh-client -C %d,%d", cols, rows))
	return err
}

// Renegotiate re-votes the client size and refreshes the window set from
// tmux's post-vote answer — the same vote-then-list order negotiate runs on a
// fresh attach, for the same reason: a snapshot painted at a stale size tears
// the moment the surface's own vote lands. The %layout-change notification is
// asynchronous, so only an explicit list after the vote reads the size tmux
// actually settled on.
func (c *Client) Renegotiate(ctx context.Context, cols, rows int) (err error) {
	// The same span name a fresh attach's negotiate carries: it is the same
	// vote-then-list round trip, and a re-attach is the half of "slow to open"
	// a search for one name has to find.
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "tmux.negotiate",
		trace.WithAttributes(attribute.Bool(attrUnsized, false)))
	defer observe.End(span, &err)

	if err := c.Resize(ctx, cols, rows); err != nil {
		return err
	}
	windows, err := c.listWindows(ctx)
	if err != nil {
		return err
	}
	c.ctrl.set(windows)
	return nil
}

func (c *Client) SelectWindow(ctx context.Context, windowID string) error {
	if err := c.requireWindow(windowID); err != nil {
		return err
	}
	_, err := c.gw.Send(ctx, "select-window -t "+windowID)
	return err
}

// NewWindow creates a window and returns its id. The tab set itself is driven
// by the %window-add notification that follows.
//
// It opens where the active pane is rather than where the session was started,
// which is what a new tab means in a terminal emulator: the window the user is
// looking at is the one they are asking for another of.
func (c *Client) NewWindow(ctx context.Context) (string, error) {
	lines, err := c.gw.Send(ctx, `new-window -c "`+currentPathFormat+`" -P -F "#{window_id}"`)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 || !validWindowID(lines[0]) {
		return "", fmt.Errorf("tmuxcc: new-window returned no window id")
	}
	return lines[0], nil
}

func (c *Client) CloseWindow(ctx context.Context, windowID string) error {
	if err := c.requireWindow(windowID); err != nil {
		return err
	}
	_, err := c.gw.Send(ctx, "kill-window -t "+windowID)
	return err
}

// SplitDirection is which way a split lays the new pane, in tmux's words: a
// horizontal split puts it to the right, a vertical one below.
type SplitDirection string

const (
	SplitHorizontal SplitDirection = "horizontal"
	SplitVertical   SplitDirection = "vertical"
)

// PaneDirection names a neighbour of a pane; the empty value names the pane
// itself.
type PaneDirection string

const (
	PaneSelf  PaneDirection = ""
	PaneLeft  PaneDirection = "left"
	PaneRight PaneDirection = "right"
	PaneUp    PaneDirection = "up"
	PaneDown  PaneDirection = "down"
)

// SplitPane splits a pane and returns the new pane's id. The layout itself
// arrives on %layout-change, and the new pane's first paint follows from the
// reconcile that triggers. It opens where the split pane is, the same way a
// new window does (ADR a-new-tab-and-a-launcher-open-where-the-terminal-s-active-pane-is).
func (c *Client) SplitPane(ctx context.Context, pane string, direction SplitDirection) (string, error) {
	if _, err := c.pane(pane); err != nil {
		return "", err
	}
	var flag string
	switch direction {
	case SplitHorizontal:
		flag = "-h"
	case SplitVertical:
		flag = "-v"
	default:
		return "", fmt.Errorf("%w: split %q", ErrInvalidDirection, direction)
	}
	lines, err := c.gw.Send(ctx, `split-window `+flag+` -t `+pane+` -c "`+currentPathFormat+`" -P -F "#{pane_id}"`)
	if err != nil {
		return "", err
	}
	if len(lines) == 0 || !validPaneID(lines[0]) {
		return "", fmt.Errorf("tmuxcc: split-window returned no pane id")
	}
	return lines[0], nil
}

// SelectPane makes a pane its window's active pane — the pane itself, or the
// neighbour direction names, which tmux picks the way its own select-pane
// bindings do. %window-pane-changed carries the answer.
func (c *Client) SelectPane(ctx context.Context, pane string, direction PaneDirection) error {
	if _, err := c.pane(pane); err != nil {
		return err
	}
	var flag string
	switch direction {
	case PaneSelf:
	case PaneLeft:
		flag = "-L "
	case PaneRight:
		flag = "-R "
	case PaneUp:
		flag = "-U "
	case PaneDown:
		flag = "-D "
	default:
		return fmt.Errorf("%w: select %q", ErrInvalidDirection, direction)
	}
	_, err := c.gw.Send(ctx, "select-pane "+flag+"-t "+pane)
	return err
}

// KillPane kills one pane. tmux closes the window with its last pane, and
// announces that as %window-close like any other close.
func (c *Client) KillPane(ctx context.Context, pane string) error {
	if _, err := c.pane(pane); err != nil {
		return err
	}
	_, err := c.gw.Send(ctx, "kill-pane -t "+pane)
	return err
}

// ResizePane sets width and/or height in cells. Zero leaves that axis
// unchanged; nonzero values must be in 1..1000. tmux adjusts the divider
// after the pane, or before it when the pane is last.
func (c *Client) ResizePane(ctx context.Context, pane string, width, height int) error {
	if _, err := c.pane(pane); err != nil {
		return err
	}
	if width == 0 && height == 0 {
		return fmt.Errorf("%w: nothing to resize", ErrInvalidSize)
	}
	cmd := "resize-pane -t " + pane
	if width != 0 {
		if width < minDimension || width > maxDimension {
			return fmt.Errorf("%w: width %d", ErrInvalidSize, width)
		}
		cmd += fmt.Sprintf(" -x %d", width)
	}
	if height != 0 {
		if height < minDimension || height > maxDimension {
			return fmt.Errorf("%w: height %d", ErrInvalidSize, height)
		}
		cmd += fmt.Sprintf(" -y %d", height)
	}
	_, err := c.gw.Send(ctx, cmd)
	return err
}

// ZoomPane toggles a pane between filling its window and its place in the
// layout. tmux makes the pane active on the way in.
func (c *Client) ZoomPane(ctx context.Context, pane string) error {
	if _, err := c.pane(pane); err != nil {
		return err
	}
	_, err := c.gw.Send(ctx, "resize-pane -Z -t "+pane)
	return err
}

// Pane is one pane of a window. Command is its foreground process — the name
// tmux's own status line shows — while PID is the process tmux started the pane
// with, so the two name the same process only while nothing that process
// started is running. PID is 0 when tmux reported one this cannot read.
type Pane struct {
	ID      string
	PID     int
	Active  bool
	Dead    bool
	Command string
}

// ListPanes reports the panes of one window. It is a live query rather than
// controller state: only a window's active pane id is modelled there, and what
// a pane is running changes without tmux announcing anything.
func (c *Client) ListPanes(ctx context.Context, windowID string) ([]Pane, error) {
	if err := c.requireWindow(windowID); err != nil {
		return nil, err
	}
	lines, err := c.gw.Send(ctx, `list-panes -t `+windowID+` -F "`+listPanesFormat+`"`)
	if err != nil {
		return nil, err
	}
	panes := make([]Pane, 0, len(lines))
	for _, line := range lines {
		p, ok := parsePaneLine(line)
		if !ok {
			c.log.Warn().Str("line", line).Msg("unparseable list-panes row")
			continue
		}
		panes = append(panes, p)
	}
	return panes, nil
}

func (c *Client) RenameWindow(ctx context.Context, windowID, name string) error {
	if err := c.requireWindow(windowID); err != nil {
		return err
	}
	quoted, err := quoteArgument(name)
	if err != nil {
		return err
	}
	_, err = c.gw.Send(ctx, "rename-window -t "+windowID+" "+quoted)
	return err
}

// MoveWindow moves windowID to position in the session's window order and
// answers with the order tmux settled on. position indexes the resulting order,
// which is what a drop on a tab strip means; the reply is authoritative because
// the order is tmux session state and every other client attached sees the move.
//
// Two tmux behaviours are load-bearing. A move is an unlink and a relink, so
// tmux announces the window being moved as closed even though it is still there
// — suppressed here, or the tab would be torn down and rebuilt blank. And tmux
// selects whatever it moves unless told not to, while -d on the window that is
// already selected deselects it: the flag has to follow the moved window, or the
// active window changes under a reorder that was never about selection.
func (c *Client) MoveWindow(ctx context.Context, windowID string, position int) ([]Window, error) {
	windows := c.ctrl.Windows()
	from := windowIndex(windows, windowID)
	if from < 0 {
		return nil, fmt.Errorf("%w: %q", ErrUnknownWindow, windowID)
	}
	if position < 0 || position >= len(windows) {
		return nil, fmt.Errorf("%w: %d outside 0..%d", ErrInvalidPosition, position, len(windows)-1)
	}
	// A move onto its own position reorders nothing but still costs tmux a whole
	// index shift, so it never reaches the server.
	if position == from {
		return windows, nil
	}

	c.holdRelink(windowID)
	defer c.releaseRelink(windowID)

	if _, err := c.gw.Send(ctx, moveWindowCommand(windows, from, position)); err != nil {
		return nil, err
	}
	// An insert shifts every index from the target up and leaves a hole where the
	// window came from, so unrenumbered a session's indices drift apart under
	// repeated reordering — and an index is how tmux's own key bindings and every
	// other attached client address a window.
	if _, err := c.gw.Send(ctx, "move-window -r"); err != nil {
		return nil, err
	}
	return c.resync(ctx)
}

func windowIndex(windows []Window, windowID string) int {
	for i, w := range windows {
		if w.ID == windowID {
			return i
		}
	}
	return -1
}

// moveWindowCommand expresses a destination index as the insertion tmux takes.
// The anchor is a window id rather than an index because the move renumbers the
// very indices it would have been read from.
func moveWindowCommand(windows []Window, from, to int) string {
	rest := make([]string, 0, len(windows)-1)
	for i, w := range windows {
		if i != from {
			rest = append(rest, w.ID)
		}
	}

	cmd := "move-window "
	if !windows[from].Active {
		cmd += "-d "
	}
	if to == 0 {
		return cmd + "-b -s " + windows[from].ID + " -t " + rest[0]
	}
	return cmd + "-a -s " + windows[from].ID + " -t " + rest[to-1]
}

// resync folds an authoritative list-windows into the window set and publishes
// what changed. tmux flushes a command's notifications before it answers the
// next one, so the snapshot this reads is the last word on what the command did.
func (c *Client) resync(ctx context.Context) ([]Window, error) {
	since := c.ctrl.mark()
	windows, err := c.listWindows(ctx)
	if err != nil {
		return nil, err
	}
	for _, ev := range c.ctrl.reconcile(windows, since) {
		c.publish(ev)
	}
	return c.ctrl.Windows(), nil
}

func (c *Client) holdRelink(windowID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.relinking[windowID]++
}

func (c *Client) releaseRelink(windowID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.relinking[windowID] <= 1 {
		delete(c.relinking, windowID)
		return
	}
	c.relinking[windowID]--
}

func (c *Client) relinkHeld(windowID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.relinking[windowID] > 0
}

// Close detaches and waits for the process to exit. Idempotent.
func (c *Client) Close(ctx context.Context) error {
	c.closeOnce.Do(func() {
		// Recorded before the detach round trip: tmux answers it with %exit and
		// closes the stream, and the reader must not race a different reason in.
		c.noteExit("detached")
		detachCtx, cancel := context.WithTimeout(ctx, detachTimeout)
		_, _ = c.gw.Send(detachCtx, "detach")
		cancel()
		c.teardown(c.exitReasonOr("detached"))
	})
	return nil
}

// awaitHandshake waits for tmux's first control-mode output. The span covers
// the wait rather than the spawn above it: exec returns as soon as the child is
// forked, so a tmux server that has to start itself is time spent here.
func (c *Client) awaitHandshake(ctx context.Context) (err error) {
	_, span := observe.StartConditionalSpan(ctx, tracer, "tmux.handshake")
	defer observe.End(span, &err)

	select {
	case <-c.handshake:
		return nil
	case <-c.readerDone:
		if serverErr := c.gw.ServerError(); serverErr != "" {
			return fmt.Errorf("tmuxcc: control stream ended before attach: %w (tmux: %s)", c.procWaitError(), serverErr)
		}
		return fmt.Errorf("tmuxcc: control stream ended before attach: %w", c.procWaitError())
	case <-ctx.Done():
		return fmt.Errorf("tmuxcc: attach handshake: %w", ctx.Err())
	}
}

// negotiate votes the opening size before it enumerates or captures anything,
// so the snapshot each window is first painted from is already the size tmux
// settled on rather than one it is about to reflow away from.
func (c *Client) negotiate(ctx context.Context, opts Options) (err error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "tmux.negotiate",
		trace.WithAttributes(attribute.Bool(attrUnsized, opts.unsized())))
	defer observe.End(span, &err)

	if !opts.unsized() {
		if _, err := c.gw.Send(ctx, fmt.Sprintf("refresh-client -C %d,%d", opts.Cols, opts.Rows)); err != nil {
			return err
		}
	}

	windows, err := c.listWindows(ctx)
	if err != nil {
		return err
	}
	c.ctrl.set(windows)

	// Only the panes the user is about to look at are painted before Attach
	// answers. A first paint is dominated by `capture-pane -e`, which costs
	// roughly 3.5us per scrollback line inside tmux, so painting every window
	// here made attach latency scale with a session's window count while the
	// renderer could only show one of them. The rest are painted immediately
	// afterwards, on the client's own lifetime, and reach the same stream in
	// the same order — see startBackgroundPaint.
	shown, deferred := c.holdPanes(windows)
	span.SetAttributes(attribute.Int(attrDeferred, len(deferred)))
	if err := c.paintShown(ctx, shown, deferred); err != nil {
		return err
	}
	c.paint.openAll()
	c.startBackgroundPaint(deferred) //nolint:contextcheck // deliberately the client's lifetime, not this request's
	return nil
}

// On failure, paintShown releases every unpainted target; otherwise its output
// remains buffered for the client's lifetime.
func (c *Client) paintShown(ctx context.Context, shown, deferred []paintTarget) error {
	for i, target := range shown {
		if err := c.firstPaint(ctx, target.pane, target.rows); err != nil {
			c.releaseRemaining(shown[i+1:])
			c.releaseRemaining(deferred)
			return err
		}
	}
	return nil
}

// activeWindow picks the window a fresh attach paints synchronously: the one
// tmux reports as active, falling back to the first paintable window so a
// session tmux has not marked one for still gets a painted pane.
func activeWindow(windows []Window) (Window, bool) {
	var fallback Window
	var found bool
	for _, w := range windows {
		if w.ActivePane == "" {
			continue
		}
		if w.Active {
			return w, true
		}
		if !found {
			fallback, found = w, true
		}
	}
	return fallback, found
}

// paintTarget is one pane and the row count its snapshot is written at: the
// pane's own height, or the window's for a zoomed pane, which tmux draws over
// the whole window while the layout still records where it goes back to.
type paintTarget struct {
	pane string
	rows int
}

// A missing layout is a %window-add placeholder; paint its active pane at
// window height until reconciliation supplies the tree.
func paintTargets(w Window) (shown, hidden []paintTarget) {
	leaves := w.Layout.Leaves()
	if len(leaves) == 0 {
		if w.ActivePane != "" {
			shown = append(shown, paintTarget{pane: w.ActivePane, rows: w.Height})
		}
		return shown, hidden
	}
	for _, leaf := range leaves {
		switch {
		case !w.Zoomed:
			shown = append(shown, paintTarget{pane: leaf.Pane, rows: leaf.Height})
		case leaf.Pane == w.ActivePane:
			shown = append(shown, paintTarget{pane: leaf.Pane, rows: w.Height})
		default:
			hidden = append(hidden, paintTarget{pane: leaf.Pane, rows: leaf.Height})
		}
	}
	return shown, hidden
}

// holdPanes holds every pane before painting so live output cannot overtake
// its snapshot. It defers panes not visible during Attach.
func (c *Client) holdPanes(windows []Window) (shown, deferred []paintTarget) {
	active, hasActive := activeWindow(windows)
	for _, w := range windows {
		visible, hidden := paintTargets(w)
		if hasActive && w.ID == active.ID {
			shown = visible
		} else {
			deferred = append(deferred, visible...)
		}
		deferred = append(deferred, hidden...)
	}
	for _, target := range shown {
		c.paint.rehold(target.pane)
	}
	for _, target := range deferred {
		c.paint.rehold(target.pane)
	}
	return shown, deferred
}

// startBackgroundPaint paints the panes a fresh attach deferred. It runs on
// the client's lifetime rather than the attach request's, because the request's
// context is cancelled the moment Attach answers — which is the whole point of
// deferring. teardown joins it, so a client cannot outlive the goroutine.
//
// Every pane is released on every path: a pane left held buffers its output for
// the life of the client and renders nothing.
func (c *Client) startBackgroundPaint(targets []paintTarget) {
	if len(targets) == 0 {
		return
	}
	c.bgPaint.Go(func() { //nolint:contextcheck // the client's lifetime, not the request's — see lifeCtx
		ctx, cancel := context.WithTimeout(c.lifeCtx, attachTimeout)
		defer cancel()
		for i, target := range targets {
			if err := c.firstPaint(ctx, target.pane, target.rows); err != nil {
				// A failure here is not fatal to the attach that already
				// answered: the pane is released unpainted, so it renders from
				// the live stream rather than staying blank forever, and the
				// window can still be repainted on demand.
				c.log.Debug().Err(err).Str("pane", target.pane).Msg("deferred first paint failed")
				if ctx.Err() != nil {
					c.releaseRemaining(targets[i+1:])
					return
				}
			}
		}
	})
}

func (c *Client) releaseRemaining(targets []paintTarget) {
	for _, target := range targets {
		c.paint.release(target.pane, nil)
	}
}

// Repaint re-runs the first paint for every window, so a caller re-attaching to
// a live client is handed the same paintable stream a fresh attach would be. A
// transport-only drop — a stalled write, a webview reload — leaves this client
// attached while taking the emulator that rendered it, and what the dropped
// stream already delivered is not in the backlog to replay: without a fresh
// snapshot the new panes open blank against a session that never stopped.
//
// The broker is reset first, which is what makes the snapshot the caller's:
// see broker.reset. The captures are bounded like an attach's own, because the
// manager runs this under the lock every other attach queues behind.
func (c *Client) Repaint(ctx context.Context) (err error) {
	// Its own span rather than a flag on the attach above it: what separates a
	// re-attach from a fresh one in a trace is which children it has. The time
	// this span holds beyond its first paint is the two joins below: a
	// deferred pass from the previous attach can still be capturing.
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "tmux.repaint")
	defer observe.End(span, &err)

	c.repaintMu.Lock()
	defer c.repaintMu.Unlock()

	// A deferred pass from the attach before this one may still be capturing.
	// Letting the two interleave would put two snapshots of the same pane on
	// the stream in an order neither chose.
	c.bgPaint.Wait()
	c.events.reset()
	return c.paintEveryWindow(ctx)
}

// resyncOverflow recovers a stream whose backlog crossed the bound rather than
// ending it. The backlog goes, the stream is marked degraded, and the first
// paint an attach runs puts every window back on screen. Reusing that path is
// what keeps the recovery non-lossy: a snapshot carries the pane's scrollback
// (ADR terminal-first-paint-carries-scrollback), so the flood the user wants to
// read survives even though the bytes that carried it did not.
//
// Both ways this can fail end the stream the way overflow always ended it. A
// resync already running means the flood refilled the backlog inside one
// capture, and a resync the broker refuses means the last one did not hold —
// neither is fixable by capturing again. A repaint that errors has lost the
// control stream it would re-establish truth from.
func (c *Client) resyncOverflow() {
	if !c.claimResync() {
		c.teardown(overflowReason)
		return
	}
	defer c.releaseResync()

	// After the claim, so a re-attach's repaint in flight delays this rather
	// than racing it — and the resync below still runs, which is what lifts the
	// state the broker is refusing output under.
	c.repaintMu.Lock()
	defer c.repaintMu.Unlock()

	c.bgPaint.Wait()
	dropped, ok := c.events.resync(time.Now())
	if !ok {
		c.teardown(overflowReason)
		return
	}
	c.log.Warn().Int("dropped_bytes", dropped).Msg("terminal backlog overflowed; resyncing the stream")
	c.publish(LifecycleChanged{Kind: LifecycleDegraded, Message: overflowReason})

	//nolint:contextcheck // deliberately the client's lifetime: no request asked for this
	if err := c.paintEveryWindow(c.lifeCtx); err != nil {
		c.log.Warn().Err(err).Msg("terminal overflow resync failed")
		c.teardown(overflowReason)
	}
}

// claimResync admits one overflow resync at a time. A second overflow while one
// is running is a flood that refilled the whole backlog inside a single
// capture, which is the case a repaint cannot answer — and letting the two run
// together would interleave their snapshots besides.
func (c *Client) claimResync() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resyncing {
		return false
	}
	c.resyncing = true
	return true
}

func (c *Client) releaseResync() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resyncing = false
}

// paintEveryWindow re-runs the first paint for every window onto whatever the
// backlog looks like now. Callers deal with the backlog first — reset for a
// re-attach, resync for an overflow — because what the snapshots supersede is
// the difference between the two.
func (c *Client) paintEveryWindow(ctx context.Context) error {
	paintCtx, cancel := context.WithTimeout(ctx, attachTimeout)
	defer cancel()

	windows := c.Windows()
	shown, deferred := c.holdPanes(windows)
	// Onto the repaint span when a re-attach opened one, onto nothing when the
	// overflow resync is what got here.
	trace.SpanFromContext(paintCtx).SetAttributes(
		attribute.Int(attrWindows, len(windows)),
		attribute.Int(attrDeferred, len(deferred)),
	)
	if err := c.paintShown(paintCtx, shown, deferred); err != nil {
		return err
	}
	c.startBackgroundPaint(deferred) //nolint:contextcheck // deliberately the client's lifetime, not this request's
	return nil
}

// firstPaint snapshots a pane and replays the live output that arrived while
// the snapshot was in flight. Every path releases the pane: one left held
// buffers its output forever.
//
// The span is the answer to "why was the terminal slow to open": capture-pane
// costs a few microseconds per scrollback line, so this is most of an attach.
// It stays bounded at one per attach because the only callers holding a
// trigger's context paint the active window alone. Every other pass runs on
// the client's lifetime, where a conditional span emits nothing.
func (c *Client) firstPaint(ctx context.Context, pane string, rows int) error {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "tmux.first-paint",
		trace.WithAttributes(attribute.Int(attrPaintRows, rows)))
	defer span.End()

	c.paint.mark(pane)
	painted, err := c.snapshot(ctx, pane, rows)
	if err != nil {
		observe.RecordError(span, err)
		c.paint.release(pane, nil)
		return err
	}
	span.SetAttributes(attribute.Int(attrPaintBytes, len(painted)))
	c.paint.release(pane, painted)
	return nil
}

// snapshot renders a pane as the byte stream an emulator replays into an empty
// grid: bounded scrollback, then the visible screen at exactly the grid's
// height, then the mouse mode and cursor.
//
// The height is what makes this safe for a pane whose program is on the
// alternate screen. An emulator pins its viewport to the last rows it was
// written, so a screen written short would seat the pane's top row somewhere
// down the viewport, and every cursor-addressed redraw the program made after
// that would land rows away from where it aimed.
//
// The cursor is read first, and after the mark rather than before it: output
// tmux produces between the two is absent from the cursor but present in the
// replay that follows the snapshot, which redraws it and carries the cursor
// where it belongs. Read before the mark, that output would be discarded as
// already snapshotted and nothing would ever correct the position.
func (c *Client) snapshot(ctx context.Context, pane string, rows int) ([]byte, error) {
	stateLines, err := c.snapshotCmd(ctx, pane, `display-message -p -t `+pane+` "`+paneStateFormat+`"`)
	if err != nil {
		return nil, err
	}
	// -J on the history and not on the screen: a scrollback row is worth
	// rejoining to the logical line it was wrapped from, so searching and
	// selecting it read as one line, but a joined screen row would re-wrap into
	// more rows than it was captured from and break the height above.
	history, err := c.snapshotCmd(ctx, pane, fmt.Sprintf("capture-pane -pe -J -S -%d -E -1 -t %s", historyLines, pane))
	if err != nil {
		return nil, err
	}
	screen, err := c.snapshotCmd(ctx, pane, "capture-pane -pe -S 0 -t "+pane)
	if err != nil {
		return nil, err
	}
	return snapshotBytes(history, screen, rows, parsePaneState(stateLines)), nil
}

// snapshotCmd runs one snapshot command. A tmux-side failure — a pane that
// closed mid-attach is the usual one — yields no lines, so the rest of the
// snapshot still paints; only a broken control stream is an error.
func (c *Client) snapshotCmd(ctx context.Context, pane, cmd string) ([]string, error) {
	lines, err := c.gw.Send(ctx, cmd)
	if err == nil {
		return lines, nil
	}
	if _, ok := errors.AsType[*CommandError](err); !ok {
		return nil, err
	}
	c.log.Warn().Ctx(ctx).Err(err).Str("pane", pane).Str("command", cmd).Msg("snapshot command skipped")
	return nil, nil
}

func (c *Client) listWindows(ctx context.Context) ([]Window, error) {
	ctx, span := observe.StartConditionalSpan(ctx, tracer, "tmux.list-windows")
	defer span.End()

	lines, err := c.gw.Send(ctx, `list-windows -F "`+listWindowsFormat+`"`)
	if err != nil {
		observe.RecordError(span, err)
		return nil, err
	}
	windows := make([]Window, 0, len(lines))
	for _, line := range lines {
		w, ok := parseWindowLine(line)
		if !ok {
			c.log.Warn().Ctx(ctx).Str("line", line).Msg("unparseable list-windows row")
			continue
		}
		windows = append(windows, w)
	}
	span.SetAttributes(attribute.Int(attrWindows, len(windows)))
	return windows, nil
}

// worker runs the commands the reader may not: the reader goroutine must never
// call the blocking Send, because the reply it would wait for arrives on that
// same goroutine.
func (c *Client) worker(ctx context.Context) {
	defer close(c.workerDone)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.reconcileReq:
			select {
			case <-c.attached:
			case <-ctx.Done():
				return
			}
			c.runReconcile(ctx)
		}
	}
}

// %window-add carries no pane ID, while %layout-change can name an uncaptured
// pane. Reconciliation snapshots either case before live output is released.
func (c *Client) runReconcile(ctx context.Context) {
	since := c.ctrl.mark()
	windows, err := c.listWindows(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.log.Warn().Err(err).Msg("window reconcile failed")
		}
		return
	}

	var unpainted []paintTarget
	for _, w := range windows {
		shown, hidden := paintTargets(w)
		for _, target := range append(shown, hidden...) {
			if c.paint.hold(target.pane) {
				unpainted = append(unpainted, target)
			}
		}
	}
	for _, ev := range c.ctrl.reconcile(windows, since) {
		c.publish(ev)
	}
	for i, target := range unpainted {
		if err := c.firstPaint(ctx, target.pane, target.rows); err != nil {
			if ctx.Err() == nil {
				c.log.Warn().Err(err).Str("pane", target.pane).Msg("first paint failed")
			}
			// hold took the gate for the whole batch; a pane this loop never
			// reaches would buffer its output for the life of the client.
			for _, unreached := range unpainted[i+1:] {
				c.paint.discard(unreached.pane)
			}
			return
		}
	}
}

// requestReconcile coalesces: a pending request absorbs later triggers.
func (c *Client) requestReconcile() {
	select {
	case c.reconcileReq <- struct{}{}:
	default:
	}
}

func (c *Client) read(stdout io.Reader) {
	defer close(c.readerDone)
	scanner := newLineScanner(stdout, maxLineBytes)
	for {
		line, err := scanner.next()
		if err != nil {
			c.gw.fail(errGatewayClosed)
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				c.log.Debug().Err(err).Msg("control stream ended")
			}
			go c.teardown(c.exitReasonOr("exited"))
			return
		}
		if err := c.gw.Feed(line); err != nil {
			c.publish(LifecycleChanged{Kind: LifecycleError, Message: err.Error()})
			go c.teardown("protocol error")
			return
		}
	}
}

func (c *Client) onNotification(n Notification) {
	switch v := n.(type) {
	case OutputNotification:
		c.paint.route(v.Pane, v.Data, time.Now())
		return
	case SessionChanged:
		c.handshakeOnce.Do(func() { close(c.handshake) })
	case WindowCloseNotification:
		// The unlink half of a move this client is running: the window is still
		// there, and MoveWindow's own resync is what reports where it went.
		if c.relinkHeld(v.Window) {
			return
		}
		// Ahead of the controller forgetting the window, which is what still
		// resolves its panes here.
		if w, ok := c.ctrl.byID(v.Window); ok {
			for _, pane := range w.Panes() {
				c.paint.discard(pane)
			}
		}
	case ExitNotification:
		c.noteExit(v.Reason)
		// tmux closes the stream right after %exit; the kill is insurance
		// against a child that does not.
		go c.terminate()
	case WindowAddNotification:
		c.requestReconcile()
	case WindowPaneChanged:
		if c.holdNewPanes(v.Window, v.Pane) {
			c.requestReconcile()
		}
	case LayoutChanged:
		c.holdNewPanes(v.Window, v.Layout.Panes()...)
		c.requestReconcile()
	}
	for _, ev := range c.ctrl.apply(n) {
		c.publish(ev)
	}
}

// holdNewPanes holds unknown panes before the controller indexes them, so live
// output cannot overtake and duplicate their first-paint snapshot.
func (c *Client) holdNewPanes(windowID string, panes ...string) bool {
	if _, ok := c.ctrl.byID(windowID); !ok {
		return false
	}
	held := false
	for _, pane := range panes {
		if _, known := c.ctrl.windowForPane(pane); !known {
			c.paint.hold(pane)
			held = true
		}
	}
	return held
}

// Every tracked pane streams, including panes hidden by zoom, so each emulator
// remains current.
//
// The measurements take c.lifeCtx because that is what they measure: this
// client's stream, for as long as it is attached.
func (c *Client) emitOutput(pane string, data []byte, at time.Time) {
	w, ok := c.ctrl.windowForPane(pane)
	if !ok {
		c.log.Debug().Str("pane", pane).Msg("output for unknown pane dropped")
		return
	}
	streamedBytes.Add(c.lifeCtx, int64(len(data)))
	if c.onEmit != nil {
		c.onEmit()
	}
	c.events.publish(Output{At: at, WindowID: w.ID, PaneID: pane, Data: data})
	bufferDepth.Record(c.lifeCtx, int64(c.events.depth()))
}

func (c *Client) publish(ev Event) {
	if lc, ok := ev.(LifecycleChanged); ok {
		switch lc.Kind {
		case LifecyclePaused:
			lifecycleTransitions.Add(c.lifeCtx, 1, statePaused)
		case LifecycleResumed:
			lifecycleTransitions.Add(c.lifeCtx, 1, stateResumed)
		default:
		}
	}
	c.events.publish(ev)
}

func (c *Client) terminate() { _ = c.proc.Kill() }

// teardown kills the child, joins the goroutines, and closes the event stream.
// Reason is the first one recorded: overflow and protocol errors beat the EOF
// that follows them.
func (c *Client) teardown(reason string) {
	c.teardownOnce.Do(func() {
		// Set before anything else: the manager reads it to decide whether the
		// client it is about to register is already dead.
		c.markTornDown(reason)
		c.cancel()
		_ = c.proc.Kill()
		<-c.readerDone
		<-c.workerDone
		// After cancel above, so a paint parked on a reply that will never
		// arrive is released by its context rather than waited out.
		c.bgPaint.Wait()
		_ = c.proc.Wait()
		c.gw.fail(errGatewayClosed)
		c.events.publish(LifecycleChanged{Kind: LifecycleExited, Message: reason})
		c.events.close()
		if c.onExit != nil {
			c.onExit(c.slug, reason)
		}
	})
}

func (c *Client) procWaitError() error {
	if err := c.proc.Wait(); err != nil {
		return err
	}
	return io.EOF
}

func (c *Client) noteExit(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exitReason == "" {
		c.exitReason = reason
	}
}

func (c *Client) markTornDown(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tornDown = true
	if c.exitReason == "" {
		c.exitReason = reason
	}
}

// exited reports the exit reason once teardown has begun. It is how the manager
// avoids registering — or handing back — a client that is already gone.
func (c *Client) exited() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.exitReason, c.tornDown
}

func (c *Client) exitReasonOr(fallback string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exitReason == "" {
		return fallback
	}
	return c.exitReason
}

// pane is the gate every pane argument passes: a pane id reaches a tmux
// command line only if it is %<digits> and a leaf of a tracked window's
// layout. The pane index behind windowForPane is additive so a departed pane's
// last output still routes; a verb on one would only earn tmux's "can't find
// pane".
func (c *Client) pane(paneID string) (Window, error) {
	if !validPaneID(paneID) {
		return Window{}, fmt.Errorf("%w: %q", ErrUnknownPane, paneID)
	}
	w, ok := c.ctrl.windowForPane(paneID)
	if !ok || !slices.Contains(w.Panes(), paneID) {
		return Window{}, fmt.Errorf("%w: %s", ErrUnknownPane, paneID)
	}
	return w, nil
}

// PaneWindow resolves a pane to the window that owns it.
func (c *Client) PaneWindow(paneID string) (Window, error) { return c.pane(paneID) }

// window is the gate every command argument passes: a window id reaches a
// tmux command line only if it is @<digits> and one this client tracks.
func (c *Client) window(windowID string) (Window, error) {
	if !validWindowID(windowID) {
		return Window{}, fmt.Errorf("%w: %q", ErrUnknownWindow, windowID)
	}
	w, ok := c.ctrl.byID(windowID)
	if !ok {
		return Window{}, fmt.Errorf("%w: %s", ErrUnknownWindow, windowID)
	}
	return w, nil
}

func (c *Client) requireWindow(windowID string) error {
	_, err := c.window(windowID)
	return err
}

func validateSize(cols, rows int) error {
	if cols < minDimension || cols > maxDimension || rows < minDimension || rows > maxDimension {
		return fmt.Errorf("%w: %dx%d", ErrInvalidSize, cols, rows)
	}
	return nil
}

// validateAttachSize also accepts 0x0, the unsized attach. Half of one is still
// invalid: a caller with a measurement has both numbers.
func validateAttachSize(cols, rows int) error {
	if cols == 0 && rows == 0 {
		return nil
	}
	return validateSize(cols, rows)
}

func platformSupported() bool {
	return buildSupportsTerminal && (runtime.GOOS == "darwin" || runtime.GOOS == "linux")
}

// parseWindowLine tolerates a malformed layout because the size and active
// pane still provide a paintable window.
func parseWindowLine(line string) (Window, bool) {
	fields := strings.SplitN(line, " ", 8)
	if len(fields) < 7 || !validWindowID(fields[0]) {
		return Window{}, false
	}
	w := Window{
		ID:         fields[0],
		Active:     fields[1] == "1",
		ActivePane: fields[2],
		Width:      atoi([]byte(fields[3])),
		Height:     atoi([]byte(fields[4])),
		Zoomed:     fields[5] == "1",
	}
	if layout, err := ParseLayout(fields[6]); err == nil {
		w.Layout = layout
	}
	if len(fields) == 8 {
		w.Name = fields[7]
	}
	return w, true
}

// parsePaneLine reads one listPanesFormat row. The pid goes through strconv
// rather than this package's own atoi, whose six-digit cap is a guard for
// notification fields and would read a Linux pid past 999999 as 0.
func parsePaneLine(line string) (Pane, bool) {
	fields := strings.SplitN(line, " ", 5)
	if len(fields) < 4 || !isPaneID([]byte(fields[0])) {
		return Pane{}, false
	}
	pid, err := strconv.Atoi(fields[1])
	if err != nil || pid < 0 {
		pid = 0
	}
	p := Pane{
		ID:     fields[0],
		PID:    pid,
		Active: fields[2] == "1",
		Dead:   fields[3] == "1",
	}
	if len(fields) == 5 {
		p.Command = fields[4]
	}
	return p, true
}

// snapshotBytes joins a captured pane into one replay: history rows, then
// exactly rows screen rows, then the modes captured cells cannot carry, and
// finally the cursor. Nothing homes or clears first — writing
// history-plus-a-full-screen scrolls the history out of the viewport on its
// own, which leaves the screen occupying the viewport exactly and the history
// reachable above it as scrollback.
func snapshotBytes(history, screen []string, rows int, state paneState) []byte {
	screen = fitRows(screen, rows)
	if len(history)+len(screen) == 0 {
		return nil
	}
	lines := make([]string, 0, len(history)+len(screen))
	lines = append(lines, history...)
	lines = append(lines, screen...)

	out := []byte(strings.Join(lines, "\r\n"))
	out = append(out, state.mouse.sequence()...)
	if state.cursor.reported {
		out = fmt.Appendf(out, "\x1b[%d;%dH", state.cursor.row+1, state.cursor.col+1)
	}
	return out
}

// fitRows holds a captured screen to the grid it is replayed into. tmux answers
// a visible-screen capture with one line per row, so this is normally a no-op;
// a short reply pads at the bottom and a long one keeps the bottom, because the
// bottom of a screen is the live end of it either way.
func fitRows(screen []string, rows int) []string {
	switch {
	case rows <= 0 || len(screen) == rows:
		return screen
	case len(screen) > rows:
		return screen[len(screen)-rows:]
	default:
		padded := make([]string, rows)
		copy(padded, screen)
		return padded
	}
}

type paneState struct {
	cursor cursor
	mouse  mouseMode
}

// cursor is a pane's cursor as tmux reports it: 0-based row and column within
// the visible screen. An unreported cursor leaves the replay's own end position
// standing rather than guessing at one.
type cursor struct {
	row, col int
	reported bool
}

// mouseMode is the terminal protocol tmux last saw the pane enable. Captured
// cells cannot carry DEC private modes, so a fresh emulator needs this restored
// before its wheel listener can report input to the program.
type mouseMode struct {
	protocol int
	sgr      bool
}

func (m mouseMode) sequence() []byte {
	if m.protocol == 0 {
		return nil
	}
	if m.sgr {
		return fmt.Appendf(nil, "\x1b[?%d;1006h", m.protocol)
	}
	return fmt.Appendf(nil, "\x1b[?%dh", m.protocol)
}

func parsePaneState(lines []string) paneState {
	if len(lines) == 0 {
		return paneState{}
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 2 || !isDigits([]byte(fields[0])) || !isDigits([]byte(fields[1])) {
		return paneState{}
	}
	state := paneState{cursor: cursor{
		row: atoi([]byte(fields[0])), col: atoi([]byte(fields[1])), reported: true,
	}}
	if len(fields) < 6 {
		return state
	}
	switch {
	case fields[4] == "1":
		state.mouse.protocol = 1003
	case fields[3] == "1":
		state.mouse.protocol = 1002
	case fields[2] == "1":
		state.mouse.protocol = 1000
	}
	state.mouse.sgr = fields[5] == "1"
	return state
}

// quoteArgument single-quotes a tmux command argument. A newline would break
// the line-oriented protocol itself, so it is refused rather than escaped.
func quoteArgument(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidName)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: control character", ErrInvalidName)
		}
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", nil
}
