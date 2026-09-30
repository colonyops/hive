package app

import (
	"context"
	"sync"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

// WebhookService owns the local webhook listener's configuration and the
// per-node capture the editor shows. The listener is nil when webhooks are
// disabled, or in mock modes without an explicit port claim; the configured
// port still describes the endpoint a live run would serve.
type WebhookService struct {
	settings *settings.Store
	captures *stores.WebhookCaptureStore
	listener *webhook.Listener
	host     string
	port     int

	mu       sync.Mutex
	startErr error
}

type WebhookDeps struct {
	Settings *settings.Store
	Captures *stores.WebhookCaptureStore
	Listener *webhook.Listener
	Host     string
	Port     int
}

func newWebhookService(d WebhookDeps) *WebhookService {
	return &WebhookService{settings: d.Settings, captures: d.Captures, listener: d.Listener, host: d.Host, port: d.Port}
}

func (s *WebhookService) setStartError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startErr = err
}

// WebhookState joins the persisted configuration with this session's running
// listener, so a caller sees what is configured and what is live in one read.
type WebhookState struct {
	Enabled bool
	Host    string
	Port    int
	PortMin int
	PortMax int
	// PortOverridden reports that the env override is in force, in which case
	// Port is the override and editing it has no effect.
	PortOverridden bool
	// Running and BoundPort describe this session's listener. BoundPort is 0
	// when it never bound.
	Running    bool
	BoundHost  string
	BoundPort  int
	StartError string
	// RestartRequired reports that the persisted configuration and the running
	// listener disagree — both toggles only take effect at startup.
	RestartRequired bool
}

// Endpoint reports the listener's live state and the port endpoints are
// served on.
func (s *WebhookService) Endpoint(context.Context) (running bool, port int) {
	if s.listener != nil && s.listener.Running() {
		return true, s.listener.Port()
	}
	return false, s.port
}

// Host reports the actual bound host when running, else the startup host.
func (s *WebhookService) Host() string {
	if s.listener != nil && s.listener.Running() {
		return s.listener.Host()
	}
	return s.host
}

// State returns the persisted configuration alongside the listener's state.
func (s *WebhookService) State(context.Context) (WebhookState, error) {
	cfg, err := s.settings.Effective()
	if err != nil {
		return WebhookState{}, Wrap(err, KindInternal, "reading settings")
	}

	enabled := cfg.HTTP.Enabled
	state := WebhookState{
		Enabled:        enabled,
		Host:           cfg.HTTP.Host,
		Port:           cfg.HTTP.Port,
		PortMin:        settings.WebhookPortMin,
		PortMax:        settings.WebhookPortMax,
		PortOverridden: cfg.EnvironmentOverridden(settings.EnvHTTPPort),
	}
	if s.listener != nil {
		state.Running = s.listener.Running()
		state.BoundHost = s.listener.Host()
		if err := s.listener.StartError(); err != nil {
			state.StartError = err.Error()
		}
	}
	s.mu.Lock()
	if s.startErr != nil {
		state.StartError = s.startErr.Error()
	}
	s.mu.Unlock()
	if state.Running {
		state.BoundPort = s.listener.Port()
	}
	portChanged := state.Port != 0 && state.BoundPort != state.Port
	state.RestartRequired = enabled != state.Running || (state.Running && (portChanged || s.host != state.Host))
	return state, nil
}

// SetState persists the enable toggle and port. Neither is applied to the
// running listener: both are startup-time decisions, and State reports the
// pending restart.
func (s *WebhookService) SetState(_ context.Context, enabled bool, host string, port int) error {
	_, err := s.settings.Update(func(current *settings.Settings) error {
		current.HTTP.Enabled = enabled
		current.HTTP.Host = host
		current.HTTP.Port = port
		if err := current.Validate(); err != nil {
			return Wrap(err, KindInvalid, "validating webhook settings")
		}
		return nil
	})
	if err == nil || KindOf(err) == KindInvalid {
		return err
	}
	return Wrap(err, KindInternal, "saving settings")
}

// GeneratePort returns a fresh random port from the generation range without
// persisting it: it is offered as a candidate, and saving is what commits it.
func (s *WebhookService) GeneratePort(ctx context.Context) (int, error) {
	port, err := settings.AllocateWebhookPort(ctx)
	return port, Wrap(err, KindUnavailable, "allocating a webhook port")
}

// WebhookCapture is one sources.webhook node's most recent delivery.
// ReceivedAt of 0 means nothing has been captured yet.
type WebhookCapture struct {
	ReceivedAt    int64
	Body          []byte
	FeedShaped    bool
	MissingFields []string
}

// Capture returns the last request body posted to a sources.webhook node, with
// the non-blocking feed-shape verdict: a payload missing feed-item fields
// ingests fine but renders minimally in feeds.
func (s *WebhookService) Capture(ctx context.Context, flowID, nodeID string) (WebhookCapture, error) {
	topic := "source:" + flowID + "/" + nodeID
	row, err := s.captures.Get(ctx, topic)
	if stores.IsNotFound(err) {
		return WebhookCapture{}, nil
	}
	if err != nil {
		return WebhookCapture{}, Wrap(err, KindInternal, "reading the webhook capture for %s", topic)
	}
	missing := webhook.MissingFeedItemFields(row.Body)
	return WebhookCapture{
		ReceivedAt:    row.ReceivedAt,
		Body:          row.Body,
		FeedShaped:    len(missing) == 0,
		MissingFields: missing,
	}, nil
}
