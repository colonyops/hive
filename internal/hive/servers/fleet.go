// Package servers is the fleet of hive servers a client works with: the
// local daemon (or engine) and each server under servers: in config.yaml,
// reached over ssh. The CLI, the TUI, and Desktop get every session API from
// one fleet, so they agree on which servers exist and how each one is.
package servers

import (
	"bytes"
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/daemon/wire"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/platform/sshdial"
	"github.com/colonyops/hive/pkg/logutils"
)

// ReadTimeout bounds each read-only call on a connection to a server, so a
// slow server cannot block a list of local sessions.
const ReadTimeout = 10 * time.Second

// ConnectTimeout bounds a read-only call that must first connect. ssh gives up
// on a dead server in 10s by itself (ConnectTimeout in sshdial), so this limit
// matters for a healthy server on a slow link and for a stuck daemon.
const ConnectTimeout = 30 * time.Second

// RiskTimeout bounds a risk check, which runs git on the whole clone.
const RiskTimeout = time.Minute

// DeleteTimeout bounds a delete on a server. It is long because a delete removes
// a whole clone, and the server finishes the delete after the client gives up.
const DeleteTimeout = 2 * time.Minute

type Fleet struct {
	local   sessionapi.API
	servers []*Server
}

// New builds a fleet over local and the configured servers. It does not
// dial: a server connects on its first call or Check. clientVersion is this
// build's version, which an incompatible server's error names.
func New(logger zerolog.Logger, local sessionapi.API, configs []config.ServerConfig, clientVersion string) *Fleet {
	logger = logutils.Component(logger, "servers")
	f := &Fleet{local: local}
	for _, cfg := range configs {
		f.servers = append(f.servers, newServer(logger, cfg, clientVersion))
	}
	return f
}

// Local is the session API for this machine: the local daemon with
// experimental.daemon on, else the engine in this process. It is nil for a
// client that starts no engine, such as the hive servers commands.
func (f *Fleet) Local() sessionapi.API {
	return f.local
}

// Servers are the configured servers in config order.
func (f *Fleet) Servers() []*Server {
	return f.servers
}

func (f *Fleet) Server(name string) (*Server, bool) {
	for _, s := range f.servers {
		if s.config.Name == name {
			return s, true
		}
	}
	return nil, false
}

// CheckAll checks every server at once and returns their statuses in config
// order.
func (f *Fleet) CheckAll(ctx context.Context) []Status {
	out := make([]Status, len(f.servers))
	var wg sync.WaitGroup
	for i, s := range f.servers {
		wg.Go(func() { out[i] = s.Check(ctx) })
	}
	wg.Wait()
	return out
}

// Close drops every server's ssh connection.
func (f *Fleet) Close() {
	for _, s := range f.servers {
		s.client.HTTPClient().CloseIdleConnections()
	}
}

// Status is a server's state after its last check.
type Status struct {
	Name    string
	SSH     string
	State   server.State
	Version string
	// Error is the reason for an offline state, with ssh's last stderr line.
	Error     string
	RoundTrip time.Duration
	CheckedAt time.Time
}

// Server is one configured server. Its client keeps one ssh connection and
// dials a new one on the next call after the connection drops.
type Server struct {
	config        config.ServerConfig
	clientVersion string
	client        *daemonclient.Client
	stderr        *tailBuffer

	mu     sync.Mutex
	status Status
}

func newServer(logger zerolog.Logger, cfg config.ServerConfig, clientVersion string) *Server {
	s := &Server{
		config:        cfg,
		clientVersion: clientVersion,
		stderr:        &tailBuffer{},
		status:        Status{Name: cfg.Name, SSH: cfg.SSH, State: server.StateConnecting},
	}
	host := cfg.Host()
	s.client = daemonclient.New(daemonclient.Options{
		Logger: logger.With().Str("server", cfg.Name).Logger(),
		// No AutoStart: dial-stdio on the server starts its daemon.
		Dialer: func(ctx context.Context) (net.Conn, error) {
			// An error from this connection must not show a line from the last
			// one.
			s.stderr.reset()
			return sshdial.DialStdio(ctx, host, s.stderr)
		},
		Name:           cfg.Name,
		FailureDetail:  s.stderr.lastLine,
		ReadTimeout:    ReadTimeout,
		ConnectTimeout: ConnectTimeout,
		RiskTimeout:    RiskTimeout,
		DeleteTimeout:  DeleteTimeout,
		CheckProtocol:  true,
		ClientVersion:  clientVersion,
	})
	return s
}

func (s *Server) Config() config.ServerConfig {
	return s.config
}

// Client is the server's session API. It also has the daemon calls that
// sessionapi.API leaves out, such as Hello and Health.
func (s *Server) Client() *daemonclient.Client {
	return s.client
}

// Status is the result of the last Check, or StateConnecting before one.
func (s *Server) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Check asks the server's daemon for its version and records the state:
// offline when it does not answer, incompatible when its protocol is outside
// the range this client supports.
func (s *Server) Check(ctx context.Context) Status {
	began := time.Now()
	info, err := s.client.Hello(ctx)
	status := Status{
		Name:      s.config.Name,
		SSH:       s.config.SSH,
		State:     server.StateConnected,
		Version:   info.Version,
		RoundTrip: time.Since(began),
		CheckedAt: time.Now(),
	}
	if err == nil {
		err = wire.CheckProtocol(info.ProtocolVersion, info.Version, s.clientVersion)
		if err != nil {
			status.State = server.StateIncompatible
			status.Error = err.Error()
		}
	} else {
		status.State = server.StateOffline
		status.Version = ""
		status.Error = err.Error()
	}
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
	return status
}

// tailBuffer keeps the last 4 KiB written to it.
type tailBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Write(p)
	if over := b.buf.Len() - 4096; over > 0 {
		b.buf.Next(over)
	}
	return len(p), nil
}

func (b *tailBuffer) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func (b *tailBuffer) lastLine() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(b.buf.String()), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
