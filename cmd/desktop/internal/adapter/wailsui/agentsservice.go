package wailsui

import (
	"context"
	"net"
	"strconv"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// AgentsTransport is what the Agents area needs that the core does not hold:
// the per-run bearer token and the path its tmux stream is mounted at. Both
// are composed in main.go and handed here (ADR terminal-transport). It shares the terminal
// surface's token and stream because a workspace session is a tmux session
// too, just not a hive one (ADR agent-workspace-sessions-are-tmux-sessions).
type AgentsTransport struct {
	Token      string
	StreamPath string
}

// AgentsAvailability gates the Chats area. Like terminal mode, it depends on
// tmux — a session is a tmux session since ADR agent-workspace-sessions-are-tmux-sessions — so Available answers
// the same question TerminalService's does.
type AgentsAvailability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

// AgentsEndpoint bootstraps the webview: control actions go to HTTPBaseURL
// with the bearer token, the data plane opens WSURL.
type AgentsEndpoint struct {
	HTTPBaseURL string `json:"httpBaseURL"`
	WSURL       string `json:"wsURL"`
	Token       string `json:"token"`
}

// AgentsService is gate and bootstrap only, mirroring TerminalService: the
// control plane and the stream both ride loopback HTTP, so neither crosses
// the Wails bridge. webhooks is here for the same reason it is in
// TerminalService's signature — WebhookService.Endpoint is the authority on
// whether the loopback server is running and on which port, which is what
// Endpoint builds its base URLs from.
type AgentsService struct {
	workspaces *app.AgentWorkspacesService
	webhooks   *app.WebhookService
	transport  AgentsTransport
}

func NewAgentsService(workspaces *app.AgentWorkspacesService, webhooks *app.WebhookService, transport AgentsTransport) *AgentsService {
	return &AgentsService{workspaces: workspaces, webhooks: webhooks, transport: transport}
}

// Available answers even when the loopback server is down, which is why it
// composes ptyterm availability with the transport's own reachability.
func (s *AgentsService) Available(ctx context.Context) AgentsAvailability {
	if err := s.workspaces.Available(ctx); err != nil {
		return AgentsAvailability{Reason: reasonFor(err)}
	}
	if _, err := s.Endpoint(ctx); err != nil {
		return AgentsAvailability{Reason: reasonFor(err)}
	}
	return AgentsAvailability{Available: true}
}

// Endpoint reports where the agent control plane is reachable, or
// KindUnavailable while the loopback server is unbound or HTTP is disabled.
func (s *AgentsService) Endpoint(ctx context.Context) (AgentsEndpoint, error) {
	if s.transport.Token == "" || s.transport.StreamPath == "" {
		return AgentsEndpoint{}, app.Errorf(app.KindUnavailable, "The agents transport is not mounted in this build.")
	}
	running, port := s.webhooks.Endpoint(ctx)
	if !running {
		return AgentsEndpoint{}, app.Errorf(app.KindUnavailable, "The local HTTP server is not running, so the Chats area has nothing to connect to. Check http.enabled in settings.yaml.")
	}
	authority := net.JoinHostPort(s.webhooks.Host(), strconv.Itoa(port))
	return AgentsEndpoint{
		HTTPBaseURL: "http://" + authority,
		WSURL:       "ws://" + authority + s.transport.StreamPath,
		Token:       s.transport.Token,
	}, nil
}
