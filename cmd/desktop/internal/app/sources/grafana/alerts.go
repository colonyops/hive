package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/canonical"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/grafana/client"
	"github.com/colonyops/hive/internal/platform/credentials"
)

type AlertsConfig struct {
	Credential string `json:"credential" yaml:"credential" jsonschema:"title=Credential,description=The connected Grafana stack to fetch as, as 'grafana/<account>'."`
	// Matchers narrow the fetch server-side. A shared stack can hold five
	// figures of active alerts, so pulling the lot and discarding it in a
	// function node is what makes this node unusable at exactly the scale an
	// alerts feed is worth having.
	Matchers []string `json:"matchers,omitempty" yaml:"matchers,omitempty" jsonschema:"title=Label matchers,description=Alertmanager label matchers, e.g. 'squad=platform' or 'severity=~critical|warning'. An alert must match every one."`
	// Interval is the floor between fetches, for a stack whose alert list is
	// expensive to pull on every tick.
	Interval connector.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The source still only runs on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
}

func (c *AlertsConfig) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	for _, matcher := range c.Matchers {
		if err := validateMatcher(matcher); err != nil {
			return err
		}
	}
	return connector.ValidateInterval("grafana alerts", c.Interval)
}

// matcherOperators are Alertmanager's label matcher operators, longest first so
// "!=" is recognized before the "=" inside it.
var matcherOperators = []string{"!=", "=~", "!~", "="}

// validateMatcher checks a matcher is addressable — a non-empty label name and
// a recognized operator — and nothing more. The endpoint owns the syntax, so
// re-implementing its value rules here would only reject matchers it accepts.
//
// The scan is left to right rather than operator by operator: a value may
// contain an operator ("path=/a!=b"), and only the leftmost one separates the
// label name from it.
func validateMatcher(matcher string) error {
	trimmed := strings.TrimSpace(matcher)
	if trimmed == "" {
		return fmt.Errorf("grafana alerts: a matcher is empty; remove the blank line")
	}
	for i := range trimmed {
		for _, op := range matcherOperators {
			if !strings.HasPrefix(trimmed[i:], op) {
				continue
			}
			if strings.TrimSpace(trimmed[:i]) == "" {
				return fmt.Errorf("grafana alerts: matcher %q has no label name before %q", matcher, op)
			}
			return nil
		}
	}
	return fmt.Errorf("grafana alerts: matcher %q needs one of =, !=, =~, !~ (e.g. %q)", matcher, "squad=platform")
}

func (c *AlertsConfig) CredentialRef() (credentials.Ref, error) {
	return parseGrafanaRef(c.Credential)
}

// AlertsDescriptor declares the alerts connector. Unlike metrics it classifies
// and confirms absence: the Alertmanager response is the complete firing set
// for the node's matchers, so an alert that leaves it is authoritatively
// resolved, not merely unseen. Editing matchers therefore reconciles items out,
// which is correct — they are no longer in the set this node claims.
var AlertsDescriptor = connector.Descriptor{
	Type:          "sources.grafana_alerts",
	Title:         "Grafana alerts source",
	ProviderTitle: "Grafana",
	Provider:      Provider,
	Mode:          connector.ModePull,
	Stability:     connector.Stable,
	Capabilities:  connector.CapClassify | connector.CapConfirmAbsence,
	NewConfig:     func() connector.Config { return &AlertsConfig{} },
}

func NewAlertsFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*AlertsConfig)
			if !ok {
				return connector.Instance{}, fmt.Errorf("grafana alerts %q: config is %T, want *grafana.AlertsConfig", node.ID(), cfg)
			}
			ref, err := config.CredentialRef()
			if err != nil {
				return connector.Instance{}, fmt.Errorf("grafana alerts %q: %w", node.ID(), err)
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
				Pull:        &alertsSource{fetcher: fetchers.For(ref), matchers: config.Matchers, topic: node.Topic()},
				Classifier:  alertsClassifier{},
				Absence:     alertsAbsence{},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
	}
}

// alertsSource polls one node's firing alerts, emitting one message per alert
// keyed by fingerprint so each alert maps to its own durable item.
type alertsSource struct {
	fetcher  *fetcher
	matchers []string
	topic    string
}

var _ connector.PullSource = (*alertsSource)(nil)

// alertPayload is what a firing alert emits: the canonical item contract
// (docs/decisions/2026-07-24-canonical-item-contract.md) filled from the alert,
// with the raw maps riding along as provider enrichment for a function node to
// route on.
//
// alertLabels, not labels, carries the map: canonical `labels` is string tags,
// so a map there is a shape no consumer of the contract can read.
type alertPayload struct {
	Title       string            `json:"title"`
	Kind        string            `json:"kind"`
	State       string            `json:"state"`
	Body        string            `json:"body,omitempty"`
	URL         string            `json:"url,omitempty"`
	Repo        string            `json:"repo,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	AlertLabels map[string]string `json:"alertLabels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	StartsAt    string            `json:"startsAt,omitempty"`
}

func (s *alertsSource) Produce(ctx context.Context, emit func(models.Msg) error) error {
	alerts, stackURL, err := s.fetcher.Alerts(ctx, s.matchers)
	if err != nil {
		return fmt.Errorf("grafana alerts: %w", err)
	}
	for _, alert := range alerts {
		body, err := json.Marshal(firingPayload(alert, stackURL))
		if err != nil {
			return fmt.Errorf("grafana alerts: encoding %q: %w", alert.Fingerprint, err)
		}
		if err := emit(models.Msg{Key: alert.Fingerprint, Topic: s.topic, SourceKind: SourceKind, Payload: body}); err != nil {
			return err
		}
	}
	return nil
}

// ItemKind is the canonical `kind` every Grafana alert item carries, both from
// the Alertmanager node and the IRM one, so `applies_to: [Alert]` targets an
// alert whichever of them produced it.
const ItemKind = "Alert"

// Grafana's own plumbing rides in the same maps as the user's labels and
// annotations, wrapped in double underscores. The connector reads the two it
// needs by name and keeps every one of them out of the tags it emits.
const (
	labelRuleUID    = "__alert_rule_uid__"
	annotationValue = "__value_string__"
)

// firingPayload builds the payload for a currently firing alert. State is
// always "firing" here — the resolved state is minted by the absence confirmer
// when the alert leaves the firing set.
func firingPayload(alert client.Alert, stackURL string) alertPayload {
	return alertPayload{
		Title:       alertTitle(alert),
		Kind:        ItemKind,
		State:       stateFiring,
		Body:        alertBody(alert),
		URL:         client.AlertRuleURL(stackURL, alert.Labels[labelRuleUID]),
		Repo:        strings.TrimSpace(alert.Labels["grafana_folder"]),
		Labels:      labelTags(alert.Labels),
		AlertLabels: alert.Labels,
		Annotations: alert.Annotations,
		StartsAt:    alert.StartsAt,
	}
}

// alertTitle is the summary annotation, else the rule name. One rule firing on
// several instances gives every instance the same summary, so the instance
// qualifies the title where the alert names one — otherwise a feed shows N
// identical rows. Presentation only: identity is the fingerprint either way.
func alertTitle(alert client.Alert) string {
	title := strings.TrimSpace(alert.Annotations["summary"])
	if title == "" {
		title = strings.TrimSpace(alert.Labels["alertname"])
	}
	if title == "" {
		title = "Grafana alert"
	}
	if instance := strings.TrimSpace(alert.Labels["instance"]); instance != "" {
		return title + " (" + instance + ")"
	}
	return title
}

// alertBody is the detail pane's markdown: the rule's description, then the
// evaluation that tripped it. Absent fields are omitted rather than rendered
// empty, so an alert carrying only a summary gets no body at all.
func alertBody(alert client.Alert) string {
	sections := make([]string, 0, 2)
	if description := strings.TrimSpace(alert.Annotations["description"]); description != "" {
		sections = append(sections, description)
	}
	facts := make([]string, 0, 3)
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			facts = append(facts, "- **"+label+"** "+value)
		}
	}
	add("Firing since", alert.StartsAt)
	add("Value", alert.Annotations[annotationValue])
	add("Runbook", alert.Annotations["runbook_url"])
	if len(facts) > 0 {
		sections = append(sections, strings.Join(facts, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

// labelTags flattens a label map to the canonical contract's string tags,
// sorted so an unchanged alert encodes byte-for-byte the same on every poll and
// the source-head comparison keeps skipping it.
func labelTags(labels map[string]string) []string {
	tags := make([]string, 0, len(labels))
	for key, value := range labels {
		if reservedLabel(key) {
			continue
		}
		tags = append(tags, key+"="+value)
	}
	if len(tags) == 0 {
		return nil
	}
	slices.Sort(tags)
	return tags
}

// reservedLabel reports whether a key is Grafana's own plumbing rather than a
// label someone attached — every reserved name is double-underscore-wrapped.
func reservedLabel(key string) bool {
	return len(key) > 4 && strings.HasPrefix(key, "__") && strings.HasSuffix(key, "__")
}

const (
	stateFiring   = "firing"
	stateResolved = "resolved"
)

// alertsClassifier maps an alert's state to lifecycle: firing is active,
// resolved is terminal.
type alertsClassifier struct{}

var _ models.Classifier = alertsClassifier{}

func (alertsClassifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	state := canonical.State(current.Payload)
	lifecycle := models.LifecycleActive
	if state == stateResolved {
		lifecycle = models.LifecycleTerminal
	}
	out := models.Classification{
		Kind:          stateFiring,
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
		out.Kind, out.Summary, out.Transition, out.ArchivedReason = stateResolved, "Resolved", models.TransitionEnteredTerminal, stateResolved
	case prev == stateResolved && state != stateResolved:
		out.Kind, out.Summary, out.Transition = stateFiring, "Firing again", models.TransitionLeftTerminal
	default:
		out.Kind, out.Attention = "updated", models.AttentionTrivial
	}
	return out
}

// alertsAbsence marks every alert that left the firing set as resolved. The
// Alertmanager response is the complete firing set, so an absent alert is
// authoritatively resolved, not merely unseen — every verdict is terminal.
type alertsAbsence struct{}

var _ models.AbsenceConfirmer = alertsAbsence{}

func (alertsAbsence) ConfirmAbsence(_ context.Context, previous []models.Observation) (map[string]models.AbsenceVerdict, error) {
	verdicts := make(map[string]models.AbsenceVerdict, len(previous))
	for _, prev := range previous {
		resolved := prev
		resolved.Payload = canonical.WithState(prev.Payload, stateResolved)
		verdicts[prev.ExternalID] = models.AbsenceVerdict{Current: &resolved, Terminal: true}
	}
	return verdicts, nil
}
