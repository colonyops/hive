package herdrcore

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// ErrClosed is returned by calls on a client whose connection has ended.
var ErrClosed = errors.New("herdrcore: connection closed")

// Client is one framed-mode connection to a core. Calls run side by side on
// channel 0; each attach stream gets its own channel.
type Client struct {
	conn  net.Conn
	hello HelloResult

	wmu    sync.Mutex
	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan rpcMessage
	streams map[uint32]*Stream
	// Frames for a channel whose attach reply has been read but whose Stream
	// is not registered yet.
	early map[uint32][]frame
	err   error

	done chan struct{}
}

// Dial connects to the core listening at socket and switches the connection
// to framed mode.
func Dial(ctx context.Context, socket string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	req, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 0, Method: "hello", Params: map[string]any{"framed": true, "generation": Generation}})
	if _, err := conn.Write(append(req, '\n')); err != nil {
		_ = conn.Close()
		return nil, err
	}
	r := bufio.NewReaderSize(conn, 64<<10)
	line, err := r.ReadBytes('\n')
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("herdrcore: hello: %w", err)
	}
	var reply rpcMessage
	if err := json.Unmarshal(line, &reply); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("herdrcore: hello: %w", err)
	}
	if reply.Error != nil {
		_ = conn.Close()
		return nil, reply.Error
	}
	c := &Client{
		conn:    conn,
		pending: map[int64]chan rpcMessage{},
		streams: map[uint32]*Stream{},
		early:   map[uint32][]frame{},
		done:    make(chan struct{}),
	}
	if err := json.Unmarshal(reply.Result, &c.hello); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("herdrcore: hello: %w", err)
	}
	_ = conn.SetDeadline(timeZero)
	go c.readLoop(r)
	return c, nil
}

// Hello is the core's handshake reply.
func (c *Client) Hello() HelloResult { return c.hello }

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// Err is why the connection ended, once Done is closed.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close ends the connection and every stream on it.
func (c *Client) Close() error { return c.conn.Close() }

// Call sends one JSON-RPC request on channel 0 and decodes its result into
// result (which may be nil).
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	if params == nil {
		params = struct{}{}
	}
	id := c.nextID.Add(1)
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return err
	}
	ch := make(chan rpcMessage, 1)
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return ErrClosed
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	if err := c.send(0, KindData, body); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClosed
	case msg := <-ch:
		if msg.Error != nil {
			return msg.Error
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(msg.Result, result)
	}
}

// Run starts a program (or the machine's shell) in a new terminal.
func (c *Client) Run(ctx context.Context, p RunParams) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.run", p, &t)
	return t, err
}

// Get returns one terminal's record.
func (c *Client) Get(ctx context.Context, terminal string) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.get", map[string]string{"terminal": terminal}, &t)
	return t, err
}

// List returns the live terminals; with all, finished ones too.
func (c *Client) List(ctx context.Context, all bool) ([]Terminal, error) {
	var res struct {
		Terminals []Terminal `json:"terminals"`
	}
	err := c.Call(ctx, "terminal.list", map[string]bool{"all": all}, &res)
	return res.Terminals, err
}

// Kill ends a terminal's program.
func (c *Client) Kill(ctx context.Context, terminal string) error {
	return c.Call(ctx, "terminal.kill", map[string]string{"terminal": terminal}, nil)
}

// Attach opens a raw stream to a live terminal. The stream's first bytes are
// the current screen (with History lines of scrollback), then live output.
func (c *Client) Attach(ctx context.Context, p AttachParams) (*Stream, AttachResult, error) {
	var res AttachResult
	if err := c.Call(ctx, "terminal.attach", p, &res); err != nil {
		return nil, res, err
	}
	s := newStream(c, res.Channel, res.AutomaticInput)
	c.mu.Lock()
	early := c.early[res.Channel]
	delete(c.early, res.Channel)
	c.streams[res.Channel] = s
	c.mu.Unlock()
	for _, f := range early {
		s.deliver(f)
	}
	return s, res, nil
}

func (c *Client) send(channel uint32, kind byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.conn.Write(encodeFrame(channel, kind, payload))
	return err
}

func (c *Client) readLoop(r io.Reader) {
	var err error
	for {
		var f frame
		f, err = readFrame(r)
		if err != nil {
			break
		}
		if f.channel == 0 {
			c.handleControl(f.payload)
			continue
		}
		c.mu.Lock()
		s := c.streams[f.channel]
		if s == nil {
			c.early[f.channel] = append(c.early[f.channel], f)
		}
		c.mu.Unlock()
		if s != nil {
			s.deliver(f)
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		err = ErrClosed
	}
	c.mu.Lock()
	c.err = err
	streams := c.streams
	c.streams = map[uint32]*Stream{}
	c.mu.Unlock()
	for _, s := range streams {
		s.end(nil, err)
	}
	close(c.done)
}

func (c *Client) handleControl(payload []byte) {
	var msg rpcMessage
	if json.Unmarshal(payload, &msg) != nil || msg.ID == nil {
		return
	}
	c.mu.Lock()
	ch := c.pending[*msg.ID]
	c.mu.Unlock()
	if ch != nil {
		ch <- msg
	}
}

func (c *Client) forget(channel uint32) {
	c.mu.Lock()
	delete(c.streams, channel)
	c.mu.Unlock()
}
