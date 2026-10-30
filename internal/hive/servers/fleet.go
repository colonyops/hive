// Package servers is the fleet of hive servers a client works with: the
// local daemon (or engine) and each server under servers: in config.yaml,
// reached over ssh. The CLI, the TUI, and Desktop get every session API from
// one fleet, so they agree on which servers exist and how each one is.
package servers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/server"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/platform/sshdial"
	"github.com/colonyops/hive/pkg/logutils"
)

// ReadTimeout bounds each read-only call to a server, so a slow server cannot
// block a list of local sessions.
const ReadTimeout = 10 * time.Second

// DeleteTimeout bounds a delete on a server. It is long because a delete removes
// a whole clone, and the server finishes the delete after the client gives up.
const DeleteTimeout = 2 * time.Minute

type Fleet struct {
	local   sessionapi.API
	servers []*Server
}

// New builds a fleet over local and the configured servers. It does not
// dial: a server connects on its first call or Check.
func New(logger zerolog.Logger, local sessionapi.API, configs []config.ServerConfig) *Fleet {
	logger = logutils.Component(logger, "servers")
	f := &Fleet{local: local}
	for _, cfg := range configs {
		f.servers = append(f.servers, newServer(logger, cfg))
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
	config config.ServerConfig
	client *daemonclient.Client
	stderr *tailBuffer

	mu     sync.Mutex
	status Status
}

func newServer(logger zerolog.Logger, cfg config.ServerConfig) *Server {
	s := &Server{
		config: cfg,
		stderr: &tailBuffer{},
		status: Status{Name: cfg.Name, SSH: cfg.SSH, State: server.StateConnecting},
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
		ReadTimeout:   ReadTimeout,
		DeleteTimeout: DeleteTimeout,
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

// Check asks the server's daemon for its version and records the state.
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
	if err != nil {
		status.State = server.StateOffline
		status.Version = ""
		status.Error = s.Explain(err).Error()
	}
	s.mu.Lock()
	s.status = status
	s.mu.Unlock()
	return status
}

// CopyStderrTo also writes ssh's stderr to w, for a CLI command whose
// errors would otherwise show only a closed connection.
func (s *Server) CopyStderrTo(w io.Writer) {
	s.stderr.mu.Lock()
	defer s.stderr.mu.Unlock()
	s.stderr.copyTo = w
}

// Explain adds ssh's last stderr line to err, since a failed ssh shows up
// only as a closed connection.
func (s *Server) Explain(err error) error {
	if line := s.stderr.lastLine(); line != "" {
		return fmt.Errorf("%w; %s", err, line)
	}
	return err
}

// tailBuffer keeps the last 4 KiB written to it.
type tailBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	copyTo io.Writer
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.copyTo != nil {
		_, _ = b.copyTo.Write(p)
	}
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
