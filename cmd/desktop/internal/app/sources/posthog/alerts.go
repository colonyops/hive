package posthog

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog/client"
)

// Normalized alert states. PostHog sends display strings — "Firing", "Not
// firing" — so every comparison here runs on the normalized form; matching the
// wire strings directly would break the first time they change the casing.
const (
	stateFiring    = "firing"
	stateNotFiring = "not_firing"
)

type AlertsConfig struct {
	Credential string `json:"credential" yaml:"credential" jsonschema:"title=Credential,description=The connected PostHog project to fetch as, as 'posthog/<account>'."`
	// FiringOnly keeps a feed to alerts that are actually breaching. It is an
	// opt-in so the zero value emits every alert, which is what makes the
	// firing→resolved transition visible on the item that was already there.
	FiringOnly bool `json:"firing_only,omitempty" yaml:"firing_only,omitempty" jsonschema:"title=Firing only,description=Emit only alerts that are currently firing. Off by default, so an alert that stops firing updates its existing item instead of vanishing."`
	// Interval is the floor between fetches, for a project whose alert list is
	// not worth pulling on every tick.
	Interval connector.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The source still only runs on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
}

func (c *AlertsConfig) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	return connector.ValidateInterval("posthog alerts", c.Interval)
}

func (c *AlertsConfig) CredentialRef() (credentials.Ref, error) {
	return parsePostHogRef(c.Credential)
}

// AlertsDescriptor declares the insight-alert connector. Like the error
// connector it classifies but does not confirm absence: the alert list carries
// every alert with its current state, so an alert that leaves it was deleted,
// not resolved — a resolution arrives as a state change on an alert that is
// still listed.
var AlertsDescriptor = connector.Descriptor{
	Type:          "sources.posthog_alerts",
	Title:         "PostHog insight alerts source",
	ProviderTitle: "PostHog",
	Provider:      Provider,
	Mode:          connector.ModePull,
	Stability:     connector.Stable,
	Capabilities:  connector.CapClassify,
	NewConfig:     func() connector.Config { return &AlertsConfig{} },
}

func NewAlertsFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*AlertsConfig)
			if !ok {
				return connector.Instance{}, fmt.Errorf("posthog alerts %q: config is %T, want *posthog.AlertsConfig", node.ID(), cfg)
			}
			ref, err := config.CredentialRef()
			if err != nil {
				return connector.Instance{}, fmt.Errorf("posthog alerts %q: %w", node.ID(), err)
			}
			return connector.Instance{
				Type: AlertsDescriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:   node.FlowID,
					SourceKind:  SourceKind,
					SourceScope: ref.Account,
					Policy:      node.Policy,
				},
				Pull:        &alertsSource{fetcher: fetchers.For(ref), firingOnly: config.FiringOnly, topic: node.Topic()},
				Classifier:  alertsClassifier{},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
	}
}

// alertsSource polls one node's insight alerts, emitting one message per alert
// keyed by its id so a firing→resolved transition updates the same item.
type alertsSource struct {
	fetcher    *fetcher
	firingOnly bool
	topic      string
}

var _ connector.PullSource = (*alertsSource)(nil)

// alertPayload is what an alert emits. threshold and condition are carried
// verbatim so a function node can route on them.
type alertPayload struct {
	ID                  string          `json:"id"`
	Kind                string          `json:"kind"`
	Title               string          `json:"title"`
	Body                string          `json:"body,omitempty"`
	URL                 string          `json:"url"`
	State               string          `json:"state"`
	UpdatedAt           int64           `json:"updatedAt,omitempty"`
	Enabled             bool            `json:"enabled"`
	Insight             string          `json:"insight,omitempty"`
	CalculationInterval string          `json:"calculationInterval,omitempty"`
	LastValue           *float64        `json:"lastValue,omitempty"`
	LastNotifiedAt      string          `json:"lastNotifiedAt,omitempty"`
	LastCheckedAt       string          `json:"lastCheckedAt,omitempty"`
	Threshold           json.RawMessage `json:"threshold,omitempty"`
	Condition           json.RawMessage `json:"condition,omitempty"`
	Project             string          `json:"project,omitempty"`
}

func (s *alertsSource) Produce(ctx context.Context, emit func(models.Msg) error) error {
	alerts, binding, err := s.fetcher.Alerts(ctx)
	if err != nil {
		return fmt.Errorf("posthog alerts: %w", err)
	}
	for _, alert := range alerts {
		state := alertState(alert.State)
		if s.firingOnly && state != stateFiring {
			continue
		}
		body, err := json.Marshal(alertPayload{
			ID:                  alert.ID,
			Kind:                AlertItemKind,
			Title:               alertTitle(alert),
			Body:                alertBody(alert, binding.Name),
			URL:                 client.InsightURL(binding.URL, binding.ProjectID, alert.Insight.ShortID),
			State:               state,
			UpdatedAt:           lastActivity(alert),
			Enabled:             alert.Enabled,
			Insight:             strings.TrimSpace(alert.Insight.Name),
			CalculationInterval: alert.CalculationInterval,
			LastValue:           alert.LastValue,
			LastNotifiedAt:      alert.LastNotifiedAt,
			LastCheckedAt:       alert.LastCheckedAt,
			Threshold:           alert.Threshold,
			Condition:           alert.Condition,
			Project:             binding.Name,
		})
		if err != nil {
			return fmt.Errorf("posthog alerts: encoding %q: %w", alert.ID, err)
		}
		if err := emit(models.Msg{Key: alert.ID, Topic: s.topic, SourceKind: SourceKind, Payload: body}); err != nil {
			return err
		}
	}
	return nil
}

// AlertItemKind is the canonical `kind` every insight-alert item carries, so
// a flow can route alerts apart from error items (`applies_to: [Alert]`).
const AlertItemKind = "Alert"

// alertBody is the detail pane's markdown: the insight the alert watches and
// the last evaluation, which is what says whether the alert is live and what
// tripped it. Absent fields are omitted rather than rendered empty.
func alertBody(alert client.Alert, project string) string {
	lines := make([]string, 0, 6)
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, "- **"+label+"** "+value)
		}
	}
	add("Insight", alert.Insight.Name)
	if alert.LastValue != nil {
		add("Last value", strconv.FormatFloat(*alert.LastValue, 'f', -1, 64))
	}
	add("Last checked", alert.LastCheckedAt)
	add("Last notified", alert.LastNotifiedAt)
	add("Checked every", alert.CalculationInterval)
	add("Project", project)
	if !alert.Enabled {
		lines = append(lines, "- **Enabled** no")
	}
	return strings.Join(lines, "\n")
}

func alertTitle(alert client.Alert) string {
	for _, candidate := range []string{alert.Name, alert.Insight.Name} {
		if title := strings.TrimSpace(candidate); title != "" {
			return title
		}
	}
	return "PostHog alert"
}

// lastActivity is when this alert last did something worth timestamping. A
// notification beats a check: an alert checked every 15 minutes would
// otherwise look freshly active on every poll.
func lastActivity(alert client.Alert) int64 {
	if stamp := epochMillis(alert.LastNotifiedAt); stamp != 0 {
		return stamp
	}
	return epochMillis(alert.LastCheckedAt)
}

// alertState normalizes PostHog's display strings ("Not firing") onto snake
// case. An unrecognized state passes through normalized rather than being
// forced to firing, so a value added later is visible instead of alarming.
func alertState(state string) string {
	normalized := strings.ToLower(strings.TrimSpace(state))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return stateNotFiring
	}
	return normalized
}

// alertsClassifier maps an alert's state to lifecycle: firing is active,
// everything else — not firing, snoozed, errored — is terminal, because none
// of them is a breach asking to be looked at.
type alertsClassifier struct{}

var _ models.Classifier = alertsClassifier{}

func (alertsClassifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	state := alertStateOf(current.Payload)
	lifecycle := models.LifecycleTerminal
	if state == stateFiring {
		lifecycle = models.LifecycleActive
	}
	out := models.Classification{
		Kind:          state,
		Transition:    models.TransitionNone,
		Attention:     models.AttentionActivity,
		Lifecycle:     lifecycle,
		SourceState:   state,
		OccurrenceKey: current.ExternalID + "@" + strconv.FormatInt(current.ObservedAt, 10),
		Summary:       current.Title,
	}
	if previous == nil {
		return out
	}
	switch prev := alertStateOf(previous.Payload); {
	case prev == stateFiring && state != stateFiring:
		out.Kind, out.Summary, out.Transition, out.ArchivedReason = state, "Resolved", models.TransitionEnteredTerminal, state
	case prev != stateFiring && state == stateFiring:
		out.Kind, out.Summary, out.Transition = stateFiring, "Firing", models.TransitionLeftTerminal
	default:
		out.Kind, out.Attention = "updated", models.AttentionTrivial
	}
	return out
}

func alertStateOf(payload []byte) string {
	_, _, state := models.CanonicalFields(payload)
	return alertState(state)
}
