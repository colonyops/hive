package wailsui

import (
	"context"
	"errors"
	"net"
	"strconv"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// TerminalTransport is what the terminal needs that the core does not hold: the
// per-run bearer token and the path its WebSocket is mounted at. Both are
// composed in main.go and handed here (ADR terminal-transport).
type TerminalTransport struct {
	Token      string
	StreamPath string
}

// TerminalAvailability is the frontend's only gate on terminal mode. Reason is
// user-facing prose: no tmux, tmux too old, a server build, or no loopback
// server to carry the transport.
type TerminalAvailability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// TerminalEndpoint bootstraps the webview: control actions go to HTTPBaseURL
// with the bearer token, the data plane opens WSURL.
type TerminalEndpoint struct {
	HTTPBaseURL string `json:"httpBaseURL"`
	WSURL       string `json:"wsURL"`
	Token       string `json:"token"`
}

// TerminalService gates terminal mode and hands the webview its transport. The
// data path itself never crosses the Wails bridge.
type TerminalService struct {
	terminals *app.TerminalsService
	webhooks  *app.WebhookService
	transport TerminalTransport
}

func NewTerminalService(terminals *app.TerminalsService, webhooks *app.WebhookService, transport TerminalTransport) *TerminalService {
	return &TerminalService{terminals: terminals, webhooks: webhooks, transport: transport}
}

// Available answers even when the loopback server is down, which is why it
// composes tmux availability with the transport's own reachability.
func (s *TerminalService) Available(ctx context.Context) TerminalAvailability {
	if err := s.terminals.Available(ctx); err != nil {
		return TerminalAvailability{Reason: reasonFor(err)}
	}
	if _, err := s.Endpoint(ctx); err != nil {
		return TerminalAvailability{Reason: reasonFor(err)}
	}
	return TerminalAvailability{Available: true}
}

// Scratch declares the scratch terminal the session tree pins above the
// repositories: the tmux session to attach to, and what to call it. It is
// constant for the run, so the tree reads it once beside the session list.
func (s *TerminalService) Scratch(ctx context.Context) app.ScratchTerminal {
	return s.terminals.Scratch(ctx)
}

// Endpoint reports where the terminal server is reachable, or KindUnavailable
// while the loopback server is unbound or HTTP is disabled.
func (s *TerminalService) Endpoint(ctx context.Context) (TerminalEndpoint, error) {
	if s.transport.Token == "" || s.transport.StreamPath == "" {
		return TerminalEndpoint{}, app.Errorf(app.KindUnavailable, "The terminal transport is not mounted in this build.")
	}
	running, port := s.webhooks.Endpoint(ctx)
	if !running {
		return TerminalEndpoint{}, app.Errorf(app.KindUnavailable, "The local HTTP server is not running, so the terminal has nothing to connect to. Check http.enabled in settings.yaml.")
	}
	authority := net.JoinHostPort(s.webhooks.Host(), strconv.Itoa(port))
	return TerminalEndpoint{
		HTTPBaseURL: "http://" + authority,
		WSURL:       "ws://" + authority + s.transport.StreamPath,
		Token:       s.transport.Token,
	}, nil
}

// reasonFor takes the user-facing message off a core error; anything else is
// reported verbatim rather than swallowed.
func reasonFor(err error) string {
	if appErr, ok := errors.AsType[*app.Error](err); ok {
		return appErr.Msg
	}
	return err.Error()
}
