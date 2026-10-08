package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana/client"
	"github.com/colonyops/hive/internal/platform/credentials"
	"github.com/colonyops/hive/pkg/duration"
)

type IRMAlertsConfig struct {
	Credential string `json:"credential" yaml:"credential" jsonschema:"title=Credential,description=The connected Grafana stack to fetch as, as 'grafana/<account>'."`
	// Integration is an IRM integration's public primary key. It is the unit a
	// squad's upstream routes hang off, so it is what pins a feed to one
	// squad's alerts.
	Integration string `json:"integration,omitempty" yaml:"integration,omitempty" jsonschema:"title=Integration,description=An IRM integration id to scope to, e.g. 'CFRPV98RPR1U8'. Empty fetches every integration."`
	Team        string `json:"team,omitempty"        yaml:"team,omitempty"        jsonschema:"title=Team,description=An IRM team id to scope to. Empty fetches every team."`
	// Interval is the floor between fetches, for a scope whose alert groups
	// are expensive to pull on every tick.
	Interval duration.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The source still only runs on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
}

func (c *IRMAlertsConfig) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	return connector.ValidateInterval("grafana irm alerts", c.Interval)
}

func (c *IRMAlertsConfig) CredentialRef() (credentials.Ref, error) {
	return parseGrafanaRef(c.Credential)
}

// IRMAlertsDescriptor declares the IRM alert groups connector — the sibling of
// the Alertmanager one, over the object on-call actually routes.
//
// It is a distinct connector rather than a mode of sources.grafana_alerts
// because the two see different sets: an alert evaluated in another Mimir and
// POSTed straight to an IRM integration never reaches the stack's Alertmanager,
// and IRM groups related alerts where the Alertmanager lists instances (ADR
// grafana-irm-alert-groups-are-a-sibling-source-node-over-the-oncall-public-api).
var IRMAlertsDescriptor = connector.Descriptor{
	Type:          "sources.grafana_irm_alerts",
	Title:         "Grafana IRM alerts source",
	ProviderTitle: "Grafana",
	Provider:      Provider,
	Mode:          connector.ModePull,
	Stability:     connector.Stable,
	Capabilities:  connector.CapClassify | connector.CapConfirmAbsence,
	NewConfig:     func() connector.Config { return &IRMAlertsConfig{} },
}

func NewIRMAlertsFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*IRMAlertsConfig)
			if !ok {
				return connector.Instance{}, fmt.Errorf("grafana irm alerts %q: config is %T, want *grafana.IRMAlertsConfig", node.ID(), cfg)
			}
			ref, err := config.CredentialRef()
			if err != nil {
				return connector.Instance{}, fmt.Errorf("grafana irm alerts %q: %w", node.ID(), err)
			}
			return connector.Instance{
				Type: IRMAlertsDescriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:   node.FlowID,
					SourceKind:  SourceKind,
					SourceScope: ref.Account,
					Policy:      node.Policy,
				},
				Pull: &irmAlertsSource{
					fetcher: fetchers.For(ref),
					query: client.AlertGroupQuery{
						States:        activeAlertGroupStates,
						IntegrationID: strings.TrimSpace(config.Integration),
						TeamID:        strings.TrimSpace(config.Team),
					},
					topic: node.Topic(),
				},
				Classifier:  irmAlertsClassifier{},
				Absence:     irmAlertsAbsence{},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
	}
}

// The lifecycle an alert group is emitted with. Upstream calls a new group
// "new"; it is renamed on the way in so both Grafana source nodes speak one
// vocabulary and a function node can route on `state` without caring which of
// them produced the item.
const (
	stateAcknowledged = "acknowledged"
	stateSilenced     = "silenced"
	upstreamStateNew  = "new"
)

// activeAlertGroupStates is the set a poll asks for. Resolved is deliberately
// absent: a group leaving this set is what mints the resolved item, so fetching
// resolved groups would only re-observe what absence already settled — and the
// upstream default window is the last 30 days, which would drag a month of
// closed alerts into the feed.
var activeAlertGroupStates = []string{upstreamStateNew, stateAcknowledged, stateSilenced}

// irmAlertsSource polls one node's alert groups, emitting one message per group
// keyed by its IRM id so each group maps to its own durable item.
type irmAlertsSource struct {
	fetcher *fetcher
	query   client.AlertGroupQuery
	topic   string
}

var _ connector.PullSource = (*irmAlertsSource)(nil)

// irmAlertPayload is what an alert group emits: the canonical item contract
// filled from the group, with the rest riding along as provider enrichment for
// a function node to route on. alertLabels carries the raw map for the same
// reason the Alertmanager node does — canonical `labels` is string tags.
type irmAlertPayload struct {
	Title          string            `json:"title"`
	Kind           string            `json:"kind"`
	State          string            `json:"state"`
	Body           string            `json:"body,omitempty"`
	URL            string            `json:"url,omitempty"`
	Labels         []string          `json:"labels,omitempty"`
	AlertLabels    map[string]string `json:"alertLabels,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	Severity       string            `json:"severity,omitempty"`
	Cluster        string            `json:"cluster,omitempty"`
	Namespace      string            `json:"namespace,omitempty"`
	Integration    string            `json:"integration,omitempty"`
	Team           string            `json:"team,omitempty"`
	AlertsCount    int               `json:"alertsCount"`
	CreatedAt      string            `json:"createdAt,omitempty"`
	AcknowledgedAt string            `json:"acknowledgedAt,omitempty"`
	SilencedAt     string            `json:"silencedAt,omitempty"`
}

func (s *irmAlertsSource) Produce(ctx context.Context, emit func(models.Msg) error) error {
	groups, err := s.fetcher.AlertGroups(ctx, s.query)
	if err != nil {
		return fmt.Errorf("grafana irm alerts: %w", err)
	}
	for _, group := range groups {
		body, err := json.Marshal(alertGroupPayload(group))
		if err != nil {
			return fmt.Errorf("grafana irm alerts: encoding %q: %w", group.ID, err)
		}
		if err := emit(models.Msg{Key: group.ID, Topic: s.topic, SourceKind: SourceKind, Payload: body}); err != nil {
			return err
		}
	}
	return nil
}

func alertGroupPayload(group client.AlertGroup) irmAlertPayload {
	details := latestAlertDetails(group.LastAlert)
	labels := group.LabelMap()
	if len(details.Labels) > 0 {
		if labels == nil {
			labels = make(map[string]string, len(details.Labels))
		}
		maps.Copy(labels, details.Labels)
	}
	title := strings.TrimSpace(group.Title)
	if title == "" {
		title = "Grafana IRM alert"
	}
	severity := labels["severity"]
	return irmAlertPayload{
		Title:          title,
		Kind:           ItemKind,
		State:          alertGroupState(group.State),
		Body:           alertGroupBody(group, labels, details),
		URL:            group.URL(),
		Labels:         labelTags(labels),
		AlertLabels:    labels,
		Annotations:    details.Annotations,
		Severity:       severity,
		Cluster:        labels["cluster"],
		Namespace:      labels["namespace"],
		Integration:    group.IntegrationID,
		Team:           group.TeamID,
		AlertsCount:    group.AlertsCount,
		CreatedAt:      group.CreatedAt,
		AcknowledgedAt: group.AcknowledgedAt,
		SilencedAt:     group.SilencedAt,
	}
}

type irmSourcePayload struct {
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	Message           string            `json:"message"`
	Alerts            []struct {
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"alerts"`
}

type irmAlertDetails struct {
	Labels      map[string]string
	Annotations map[string]string
	Message     string
}

// latestAlertDetails reads the source's own context from the last alert that
// the public group listing embeds. Alertmanager sends common maps; simpler
// integrations send top-level maps, and a one-alert payload may only carry
// them on the alert itself.
func latestAlertDetails(alert *client.IRMAlert) irmAlertDetails {
	if alert == nil || len(alert.Payload) == 0 {
		return irmAlertDetails{}
	}
	var payload irmSourcePayload
	if err := json.Unmarshal(alert.Payload, &payload); err != nil {
		return irmAlertDetails{}
	}
	labels := payload.CommonLabels
	if len(labels) == 0 {
		labels = payload.Labels
	}
	annotations := payload.CommonAnnotations
	if len(annotations) == 0 {
		annotations = payload.Annotations
	}
	if len(payload.Alerts) == 1 {
		if len(labels) == 0 {
			labels = payload.Alerts[0].Labels
		}
		if len(annotations) == 0 {
			annotations = payload.Alerts[0].Annotations
		}
	}
	return irmAlertDetails{Labels: labels, Annotations: annotations, Message: payload.Message}
}

// alertGroupBody puts the source alert's explanation and labels before IRM's
// triage facts, so the detail pane answers what is firing before how IRM has
// handled the group.
func alertGroupBody(group client.AlertGroup, labels map[string]string, details irmAlertDetails) string {
	sections := make([]string, 0, 3)
	description := strings.TrimSpace(details.Annotations["description"])
	if description == "" {
		description = strings.TrimSpace(details.Message)
	}
	if description != "" {
		sections = append(sections, description)
	}
	if len(labels) > 0 {
		keys := make([]string, 0, len(labels))
		for key := range labels {
			if !reservedLabel(key) {
				keys = append(keys, key)
			}
		}
		slices.Sort(keys)
		if len(keys) > 0 {
			lines := make([]string, 0, len(keys))
			for _, key := range keys {
				lines = append(lines, key+" = "+strconv.Quote(labels[key]))
			}
			sections = append(sections, "**Alert labels**\n\n```text\n"+strings.Join(lines, "\n")+"\n```")
		}
	}
	facts := make([]string, 0, 5)
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			facts = append(facts, "- **"+label+"** "+value)
		}
	}
	if group.AlertsCount > 0 {
		add("Alerts in group", strconv.Itoa(group.AlertsCount))
	}
	add("Firing since", group.CreatedAt)
	add("Acknowledged", group.AcknowledgedAt)
	add("Silenced", group.SilencedAt)
	if len(facts) > 0 {
		sections = append(sections, strings.Join(facts, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

// alertGroupState maps an upstream state onto the emitted vocabulary. An
// unrecognized state passes through rather than being forced to firing: a state
// this build has not seen is better read literally than mislabelled.
func alertGroupState(state string) string {
	normalized := strings.ToLower(strings.TrimSpace(state))
	if normalized == upstreamStateNew || normalized == "" {
		return stateFiring
	}
	return normalized
}

// irmAlertsClassifier maps an alert group's state to lifecycle. Resolved is
// terminal; everything else is active.
//
// Unlike the Alertmanager classifier, a state change between two active states
// is real activity rather than noise: acknowledged means a human picked the
// alert up, which is the triage signal this connector exists to carry. Only a
// re-observation at an unchanged state is trivial.
type irmAlertsClassifier struct{}

var _ models.Classifier = irmAlertsClassifier{}

func (irmAlertsClassifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	state := canonical.State(current.Payload)
	lifecycle := models.LifecycleActive
	if state == stateResolved {
		lifecycle = models.LifecycleTerminal
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
	switch prev := canonical.State(previous.Payload); {
	case prev != stateResolved && state == stateResolved:
		out.Summary, out.Transition, out.ArchivedReason = "Resolved", models.TransitionEnteredTerminal, stateResolved
	case prev == stateResolved && state != stateResolved:
		out.Summary, out.Transition = "Firing again", models.TransitionLeftTerminal
	case prev != state:
		out.Summary = alertGroupSummary(state)
	default:
		out.Kind, out.Attention = "updated", models.AttentionTrivial
	}
	return out
}

func alertGroupSummary(state string) string {
	switch state {
	case stateAcknowledged:
		return "Acknowledged"
	case stateSilenced:
		return "Silenced"
	case stateFiring:
		return "Firing"
	default:
		return "State changed"
	}
}

// irmAlertsAbsence marks every alert group that left the active set as
// resolved. The listing is the complete active set for the node's scope, so an
// absent group is authoritatively resolved rather than merely unseen — every
// verdict is terminal.
type irmAlertsAbsence struct{}

var _ models.AbsenceConfirmer = irmAlertsAbsence{}

func (irmAlertsAbsence) ConfirmAbsence(_ context.Context, previous []models.Observation) (map[string]models.AbsenceVerdict, error) {
	verdicts := make(map[string]models.AbsenceVerdict, len(previous))
	for _, prev := range previous {
		resolved := prev
		resolved.Payload = canonical.WithState(prev.Payload, stateResolved)
		verdicts[prev.ExternalID] = models.AbsenceVerdict{Current: &resolved, Terminal: true}
	}
	return verdicts, nil
}
