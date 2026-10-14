package remoteconn

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/colonyops/hive/pkg/randid"
)

type Response struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}
type Batch struct {
	Frames []string `json:"frames"`
	Ended  bool     `json:"ended"`
}
type stream struct {
	socket  *websocket.Conn
	frames  chan string
	cancel  context.CancelFunc
	writeMu sync.Mutex
}
type connection struct {
	loadCredentials func(context.Context, Profile) (Credentials, *http.Client, error)
	profile         Profile
	credentials     Credentials
	client          *http.Client
	streams         map[string]*stream
	done            chan struct{}
	closeOnce       sync.Once
	mu              sync.Mutex
	refreshMu       sync.Mutex
}
type Manager struct {
	mu          sync.Mutex
	connections map[string]*connection
	closed      bool
	done        chan struct{}
}

func NewManager() *Manager {
	return &Manager{connections: map[string]*connection{}, done: make(chan struct{})}
}

func clientFor(p Profile, credentials Credentials) (*http.Client, error) {
	u, err := url.Parse(credentials.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || credentials.Token == "" {
		return nil, fmt.Errorf("remote connection requires an HTTPS URL, token, and certificate")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() || u.Port() == "" {
		return nil, fmt.Errorf("remote HTTPS endpoint must use a loopback IP and explicit port")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(credentials.Certificate)) {
		return nil, fmt.Errorf("remote certificate is missing or invalid")
	}
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         func(ctx context.Context, _, _ string) (net.Conn, error) { return dialSSH(ctx, p, u.Host) },
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: u.Hostname()},
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: time.Minute,
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func credentialsFor(ctx context.Context, p Profile) (Credentials, *http.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := bootstrap(ctx, p)
	if err != nil {
		return Credentials{}, nil, err
	}
	var credentials Credentials
	if err = json.Unmarshal(data, &credentials); err != nil {
		return Credentials{}, nil, fmt.Errorf("invalid remote connection file")
	}
	client, err := clientFor(p, credentials)
	return credentials, client, err
}

func (m *Manager) Connect(ctx context.Context, p Profile) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	go func() {
		select {
		case <-m.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	if err := p.Validate(); err != nil {
		return "", err
	}
	credentials, client, err := credentialsFor(ctx, p)
	if err != nil {
		return "", err
	}
	c := &connection{loadCredentials: credentialsFor, profile: p, credentials: credentials, client: client, streams: map[string]*stream{}, done: make(chan struct{})}
	response, err := c.request(ctx, "/api/terminal/remote/sessions", "{}")
	if err == nil && response.Status != http.StatusOK {
		err = fmt.Errorf("remote discovery failed (HTTP %d)", response.Status)
	}
	if err == nil {
		var info struct {
			ProtocolVersion     int    `json:"protocolVersion"`
			TerminalWireVersion string `json:"terminalWireVersion"`
		}
		if json.Unmarshal([]byte(response.Body), &info) != nil || info.ProtocolVersion != 1 || info.TerminalWireVersion != "2" {
			err = fmt.Errorf("remote Hive protocol is incompatible; update both installations")
		}
	}
	if err != nil {
		c.close()
		return "", err
	}
	id := randid.UUIDv7()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		c.close()
		return "", fmt.Errorf("remote manager is closed")
	}
	m.connections[id] = c
	return id, nil
}

func (m *Manager) get(id string) (*connection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.connections[id]
	if c == nil {
		return nil, fmt.Errorf("remote connection is closed")
	}
	return c, nil
}

func (c *connection) snapshot() (Credentials, *http.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.credentials, c.client
}

func (c *connection) refresh(ctx context.Context, token string) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	current, _ := c.snapshot()
	if current.Token != token {
		return nil
	}
	credentials, client, err := c.loadCredentials(ctx, c.profile)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed() {
		client.CloseIdleConnections()
		return fmt.Errorf("remote connection is closed")
	}
	c.client.CloseIdleConnections()
	c.credentials = credentials
	c.client = client
	return nil
}

func (c *connection) request(ctx context.Context, path, body string) (Response, error) {
	if !strings.HasPrefix(path, "/api/terminal/") || strings.ContainsAny(path, "?#%\\") || strings.Contains(path, "..") {
		return Response{}, fmt.Errorf("unsupported remote API path")
	}
	if len(body) > 8<<20 {
		return Response{}, fmt.Errorf("remote request exceeds 8 MiB")
	}
	ctx, cancel := c.requestContext(ctx, 30*time.Second)
	defer cancel()
	for attempt := range 2 {
		credentials, client := c.snapshot()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(credentials.URL, "/")+path, bytes.NewBufferString(body))
		if err != nil {
			return Response{}, err
		}
		req.Header.Set("Authorization", "Bearer "+credentials.Token)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return Response{}, fmt.Errorf("remote HTTPS request failed: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, (8<<20)+1))
		_ = res.Body.Close()
		if readErr != nil {
			return Response{}, readErr
		}
		if len(data) > 8<<20 {
			return Response{}, fmt.Errorf("remote response exceeds 8 MiB")
		}
		if res.StatusCode == http.StatusUnauthorized && attempt == 0 {
			if err = c.refresh(ctx, credentials.Token); err != nil {
				return Response{}, err
			}
			continue
		}
		return Response{Status: res.StatusCode, Body: string(data)}, nil
	}
	return Response{}, fmt.Errorf("remote authentication failed")
}

func (m *Manager) Request(ctx context.Context, id, path, body string) (Response, error) {
	c, err := m.get(id)
	if err != nil {
		return Response{}, err
	}
	return c.request(ctx, path, body)
}

func (m *Manager) OpenStream(ctx context.Context, id, slug string) (string, error) {
	c, err := m.get(id)
	if err != nil {
		return "", err
	}
	credentials, client := c.snapshot()
	u, err := url.Parse(credentials.URL)
	if err != nil {
		return "", err
	}
	u.Scheme = "wss"
	u.Path = "/api/terminal/stream"
	q := url.Values{"v": {"2"}, "slug": {slug}}
	u.RawQuery = q.Encode()
	dialCtx, cancelDial := c.requestContext(ctx, 15*time.Second)
	defer cancelDial()
	ws, res, err := websocket.Dial(dialCtx, u.String(), &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Authorization": {"Bearer " + credentials.Token}}})
	if err != nil {
		if res != nil && res.Body != nil {
			_ = res.Body.Close()
		}
		return "", fmt.Errorf("remote WSS connection failed")
	}
	ws.SetReadLimit(1 << 20)
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &stream{socket: ws, frames: make(chan string, 32), cancel: cancel}
	streamID := randid.UUIDv7()
	c.mu.Lock()
	if c.closed() {
		c.mu.Unlock()
		cancel()
		_ = ws.CloseNow()
		return "", fmt.Errorf("remote connection is closed")
	}
	c.streams[streamID] = s
	c.mu.Unlock()
	go func() {
		defer close(s.frames)
		defer func() { _ = ws.CloseNow() }()
		for {
			kind, data, err := ws.Read(streamCtx)
			if err != nil {
				return
			}
			if kind != websocket.MessageBinary {
				return
			}
			select {
			case s.frames <- base64.StdEncoding.EncodeToString(data):
			case <-streamCtx.Done():
				return
			}
		}
	}()
	return streamID, nil
}

func (m *Manager) getStream(id, streamID string) (*stream, error) {
	c, err := m.get(id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.streams[streamID]
	if s == nil {
		return nil, fmt.Errorf("remote stream is closed")
	}
	return s, nil
}

func (m *Manager) ReadStream(ctx context.Context, id, streamID string) (Batch, error) {
	s, err := m.getStream(id, streamID)
	if err != nil {
		return Batch{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	batch := Batch{Frames: []string{}}
	select {
	case frame, ok := <-s.frames:
		if !ok {
			batch.Ended = true
			return batch, nil
		}
		batch.Frames = append(batch.Frames, frame)
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return batch, nil
		}
		return batch, ctx.Err()
	}
	for len(batch.Frames) < 32 {
		select {
		case frame, ok := <-s.frames:
			if !ok {
				batch.Ended = true
				return batch, nil
			}
			batch.Frames = append(batch.Frames, frame)
		default:
			return batch, nil
		}
	}
	return batch, nil
}

func (m *Manager) WriteStream(ctx context.Context, id, streamID, frame string) error {
	s, err := m.getStream(id, streamID)
	if err != nil {
		return err
	}
	if len(frame) > 5500 {
		return fmt.Errorf("remote input frame is too large")
	}
	data, err := base64.StdEncoding.DecodeString(frame)
	if err != nil {
		return fmt.Errorf("invalid remote input frame")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.socket.Write(ctx, websocket.MessageBinary, data)
}

func (m *Manager) CloseStream(id, streamID string) error {
	c, err := m.get(id)
	if err != nil {
		return nil
	}
	c.mu.Lock()
	s := c.streams[streamID]
	delete(c.streams, streamID)
	c.mu.Unlock()
	if s != nil {
		s.cancel()
		return s.socket.CloseNow()
	}
	return nil
}

func (c *connection) closed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *connection) requestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	go func() {
		select {
		case <-c.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (c *connection) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.client.CloseIdleConnections()
		for id, s := range c.streams {
			s.cancel()
			_ = s.socket.CloseNow()
			delete(c.streams, id)
		}
	})
}

func (m *Manager) Disconnect(id string) {
	m.mu.Lock()
	c := m.connections[id]
	delete(m.connections, id)
	m.mu.Unlock()
	if c != nil {
		c.close()
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	if !m.closed {
		close(m.done)
	}
	m.closed = true
	connections := m.connections
	m.connections = map[string]*connection{}
	m.mu.Unlock()
	for _, c := range connections {
		c.close()
	}
}
