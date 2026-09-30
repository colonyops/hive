package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// WebhookService exposes the local webhook listener's endpoint info, its
// user-tunable settings, and each sources.webhook node's last captured
// delivery to the frontend.
type WebhookService struct {
	webhooks *app.WebhookService
}

func NewWebhookService(w *app.WebhookService) *WebhookService {
	return &WebhookService{webhooks: w}
}

// WebhookInfo describes the local webhook listener for the node editor.
type WebhookInfo struct {
	Running bool   `json:"running"`
	Port    int    `json:"port"`
	BaseURL string `json:"baseUrl"`
}

// Info returns the listener's state and the base URL sources.webhook paths are
// served under (endpoint URL = BaseURL + node path).
func (s *WebhookService) Info(ctx context.Context) WebhookInfo {
	running, port := s.webhooks.Endpoint(ctx)
	return WebhookInfo{Running: running, Port: port, BaseURL: app.WebhookBaseURLAt(s.webhooks.Host(), port)}
}

// WebhookSettings is the listener's editable configuration joined with the
// running listener's actual state, so the settings pane can show what is
// configured and what is live in one read.
type WebhookSettings struct {
	// Enabled and Port are the persisted configuration.
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	// PortMin and PortMax bound generated ports; the frontend reuses them to
	// label the field rather than restating the range.
	PortMin int `json:"portMin"`
	PortMax int `json:"portMax"`
	// PortOverridden reports that HIVE_DESKTOP_WEBHOOKS_PORT is in force, in
	// which case Port is the override and editing it has no effect.
	PortOverridden bool `json:"portOverridden"`
	// Running, BoundPort, BaseURL, and StartError describe this session's
	// listener. BoundPort is 0 when it never bound.
	Running    bool   `json:"running"`
	BoundHost  string `json:"boundHost"`
	BoundPort  int    `json:"boundPort"`
	BaseURL    string `json:"baseUrl"`
	StartError string `json:"startError"`
	// RestartRequired reports that the persisted configuration and the running
	// listener disagree — both toggles only take effect at startup.
	RestartRequired bool `json:"restartRequired"`
}

func (s *WebhookService) Settings(ctx context.Context) (WebhookSettings, error) {
	state, err := s.webhooks.State(ctx)
	if err != nil {
		return WebhookSettings{}, err
	}
	view := WebhookSettings{
		Enabled:         state.Enabled,
		Host:            state.Host,
		Port:            state.Port,
		PortMin:         state.PortMin,
		PortMax:         state.PortMax,
		PortOverridden:  state.PortOverridden,
		Running:         state.Running,
		BoundHost:       state.BoundHost,
		BoundPort:       state.BoundPort,
		StartError:      state.StartError,
		RestartRequired: state.RestartRequired,
	}
	view.BaseURL = app.WebhookBaseURLAt(state.Host, state.Port)
	if view.Running {
		view.BaseURL = app.WebhookBaseURLAt(view.BoundHost, view.BoundPort)
	}
	return view, nil
}

// SetSettings persists the enable toggle and port.
func (s *WebhookService) SetSettings(ctx context.Context, next WebhookSettings) error {
	return s.webhooks.SetState(ctx, next.Enabled, next.Host, next.Port)
}

// GeneratePort returns a fresh random port without persisting it: the
// settings pane offers it as a candidate, and saving is what commits it.
func (s *WebhookService) GeneratePort(ctx context.Context) (int, error) {
	return s.webhooks.GeneratePort(ctx)
}

// WebhookCaptureView is one sources.webhook node's most recent delivery.
// ReceivedAt of 0 means no delivery has been captured yet.
type WebhookCaptureView struct {
	ReceivedAt    int64    `json:"receivedAt"`
	Body          string   `json:"body"`
	FeedShaped    bool     `json:"feedShaped"`
	MissingFields []string `json:"missingFields"`
}

// Capture returns the last request body POSTed to a sources.webhook node, with
// the non-blocking feed-shape verdict the editor surfaces.
func (s *WebhookService) Capture(ctx context.Context, flowID, nodeID string) (WebhookCaptureView, error) {
	capture, err := s.webhooks.Capture(ctx, flowID, nodeID)
	if err != nil {
		return WebhookCaptureView{}, err
	}
	return WebhookCaptureView{
		ReceivedAt:    capture.ReceivedAt,
		Body:          string(capture.Body),
		FeedShaped:    capture.FeedShaped,
		MissingFields: capture.MissingFields,
	}, nil
}
