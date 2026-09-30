package posthog

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/posthog/client"
)

// ItemKind is the canonical `kind` every error item carries. Without one an
// item is untyped and an action can only target it as the catch-all `Item`;
// naming it lets a flow route errors specifically (`applies_to: [Error]`).
const ItemKind = "Error"

// Issue statuses. The first three are what a client may set and what the query
// returns today; archived and pending_release are legacy values PostHog still
// reads back, so they are matched rather than assumed absent.
const (
	statusActive     = "active"
	statusResolved   = "resolved"
	statusSuppressed = "suppressed"
	statusArchived   = "archived"
	statusAll        = "all"
)

// Issue orderings the query endpoint accepts.
const (
	orderLastSeen    = "last_seen"
	orderFirstSeen   = "first_seen"
	orderOccurrences = "occurrences"
	orderUsers       = "users"
	orderSessions    = "sessions"
)

const (
	defaultDateFrom = "-7d"
	defaultLimit    = 25
	maxLimit        = 100
)

// ErrorsConfig is a PostHog error-tracking source node's configuration.
// credential is a direct field, not promoted from an embedded base, because
// the provider-enforcement test scans a config's own tagged fields for it.
// There is deliberately no host or project field — both are bound to the
// account at connect time, so a node cannot pair a key with an arbitrary
// project.
type ErrorsConfig struct {
	// A ref and never a token: flows/ is dotfiles-managed, so an embedded
	// token would be a token in a git repo.
	Credential string `json:"credential"          yaml:"credential"          jsonschema:"title=Credential,description=The connected PostHog project to fetch as, as 'posthog/<account>'."`
	Status     string `json:"status,omitempty"    yaml:"status,omitempty"    jsonschema:"title=Status,enum=active,enum=resolved,enum=suppressed,enum=all,description=Which issues to fetch. Defaults to active."`
	OrderBy    string `json:"order_by,omitempty"  yaml:"order_by,omitempty"  jsonschema:"title=Order by,enum=last_seen,enum=first_seen,enum=occurrences,enum=users,enum=sessions,description=How issues are ranked before the limit is applied. Defaults to last_seen."`
	DateFrom   string `json:"date_from,omitempty" yaml:"date_from,omitempty" jsonschema:"title=Date from,description=Start of the window aggregate counts cover, as a PostHog relative date such as '-7d' or '-24h'. Defaults to -7d."`
	Limit      int    `json:"limit,omitempty"     yaml:"limit,omitempty"     jsonschema:"title=Limit,minimum=0,maximum=100,description=Maximum issues per poll, ranked by the ordering above. 0 uses the default of 25."`
	// IncludeTestAccounts is phrased as an opt-in so the Go zero value is
	// PostHog's own default of excluding internal traffic.
	IncludeTestAccounts bool `json:"include_test_accounts,omitempty" yaml:"include_test_accounts,omitempty" jsonschema:"title=Include test accounts,description=Include traffic PostHog classifies as internal or test. Off by default."`
	// Interval is the floor between fetches, for a project whose issue query
	// is not worth running on every tick.
	Interval connector.Duration `json:"interval,omitempty" yaml:"interval,omitempty" jsonschema:"title=Minimum interval,description=Shortest time between fetches. The source still only runs on a poll tick so the real cadence rounds up to the next one; empty fetches on every tick."`
}

func (c *ErrorsConfig) Validate() error {
	if _, err := c.CredentialRef(); err != nil {
		return err
	}
	switch c.Status {
	case "", statusActive, statusResolved, statusSuppressed, statusAll:
	default:
		return fmt.Errorf("posthog errors: unknown status %q (want %q, %q, %q or %q)",
			c.Status, statusActive, statusResolved, statusSuppressed, statusAll)
	}
	switch c.OrderBy {
	case "", orderLastSeen, orderFirstSeen, orderOccurrences, orderUsers, orderSessions:
	default:
		return fmt.Errorf("posthog errors: unknown order_by %q", c.OrderBy)
	}
	if c.Limit < 0 {
		return fmt.Errorf("posthog errors: limit must not be negative")
	}
	if c.Limit > maxLimit {
		return fmt.Errorf("posthog errors: limit %d exceeds the query page cap of %d", c.Limit, maxLimit)
	}
	return connector.ValidateInterval("posthog errors", c.Interval)
}

func (c *ErrorsConfig) CredentialRef() (credentials.Ref, error) {
	return parsePostHogRef(c.Credential)
}

func (c *ErrorsConfig) request() client.IssuesRequest {
	status := c.Status
	if status == "" {
		status = statusActive
	}
	orderBy := c.OrderBy
	if orderBy == "" {
		orderBy = orderLastSeen
	}
	dateFrom := strings.TrimSpace(c.DateFrom)
	if dateFrom == "" {
		dateFrom = defaultDateFrom
	}
	limit := c.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	return client.IssuesRequest{
		DateRange:          client.DateRange{DateFrom: dateFrom},
		Status:             status,
		OrderBy:            orderBy,
		OrderDirection:     "DESC",
		Limit:              limit,
		FilterTestAccounts: !c.IncludeTestAccounts,
	}
}

// ErrorsDescriptor declares the error-tracking connector. It classifies but
// does not confirm absence: the query is filtered by status, bounded by a date
// window and capped by a limit, so an issue leaving the result set may merely
// have aged out or been ranked below the cut. Treating that as "resolved" — as
// the Grafana alerts connector legitimately does with a complete firing set —
// would archive live issues.
var ErrorsDescriptor = connector.Descriptor{
	Type:          "sources.posthog_errors",
	Title:         "PostHog error tracking source",
	ProviderTitle: "PostHog",
	Provider:      Provider,
	Mode:          connector.ModePull,
	Stability:     connector.Stable,
	Capabilities:  connector.CapClassify,
	NewConfig:     func() connector.Config { return &ErrorsConfig{} },
}

func NewErrorsFactory(fetchers *Fetchers) connector.Factory {
	return connector.Factory{
		New: func(node connector.Node, cfg connector.Config) (connector.Instance, error) {
			config, ok := cfg.(*ErrorsConfig)
			if !ok {
				return connector.Instance{}, fmt.Errorf("posthog errors %q: config is %T, want *posthog.ErrorsConfig", node.ID(), cfg)
			}
			ref, err := config.CredentialRef()
			if err != nil {
				return connector.Instance{}, fmt.Errorf("posthog errors %q: %w", node.ID(), err)
			}
			return connector.Instance{
				Type: ErrorsDescriptor.Type,
				Node: node,
				Metadata: connector.Metadata{
					ProfileID:  node.FlowID,
					SourceKind: SourceKind,
					// Scope by account so several PostHog projects in one flow stay distinct.
					SourceScope: ref.Account,
					Policy:      node.Policy,
				},
				Pull:        &errorsSource{fetcher: fetchers.For(ref), request: config.request(), topic: node.Topic()},
				Classifier:  errorsClassifier{},
				MinInterval: config.Interval.Duration(),
				Config:      config,
			}, nil
		},
	}
}

// errorsSource polls one node's issues, emitting one message per issue keyed
// by issue id. Keying on the issue is the roll-up: PostHog has already grouped
// every occurrence of one exception under it, so a spike of ten thousand
// events updates one feed item instead of flooding the inbox.
type errorsSource struct {
	fetcher *fetcher
	request client.IssuesRequest
	topic   string
}

var _ connector.PullSource = (*errorsSource)(nil)

// issuePayload is what an issue emits. title, url, state and updatedAt are the
// canonical fields the ingest boundary and classifier read; the counts and
// library ride along for a function node to route on.
type issuePayload struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Body        string  `json:"body,omitempty"`
	URL         string  `json:"url"`
	State       string  `json:"state"`
	UpdatedAt   int64   `json:"updatedAt,omitempty"`
	Description string  `json:"description,omitempty"`
	Library     string  `json:"library,omitempty"`
	Source      string  `json:"source,omitempty"`
	FirstSeen   string  `json:"firstSeen,omitempty"`
	LastSeen    string  `json:"lastSeen,omitempty"`
	Occurrences float64 `json:"occurrences,omitempty"`
	Users       float64 `json:"users,omitempty"`
	Sessions    float64 `json:"sessions,omitempty"`
	Project     string  `json:"project,omitempty"`
}

func (s *errorsSource) Produce(ctx context.Context, emit func(models.Msg) error) error {
	issues, binding, err := s.fetcher.Issues(ctx, s.request)
	if err != nil {
		return fmt.Errorf("posthog errors: %w", err)
	}
	for _, issue := range issues {
		body, err := json.Marshal(issuePayload{
			ID:          issue.ID,
			Kind:        ItemKind,
			Title:       issueTitle(issue),
			Body:        issueBody(issue, binding.Name),
			URL:         client.IssueURL(binding.URL, binding.ProjectID, issue.ID),
			State:       issueState(issue.Status),
			UpdatedAt:   epochMillis(issue.LastSeen),
			Description: strings.TrimSpace(issue.Description),
			Library:     issue.Library,
			Source:      strings.TrimSpace(issue.Source),
			FirstSeen:   issue.FirstSeen,
			LastSeen:    issue.LastSeen,
			Occurrences: aggregate(issue.Aggregations, func(a client.Aggregations) float64 { return a.Occurrences }),
			Users:       aggregate(issue.Aggregations, func(a client.Aggregations) float64 { return a.Users }),
			Sessions:    aggregate(issue.Aggregations, func(a client.Aggregations) float64 { return a.Sessions }),
			Project:     binding.Name,
		})
		if err != nil {
			return fmt.Errorf("posthog errors: encoding %q: %w", issue.ID, err)
		}
		if err := emit(models.Msg{Key: issue.ID, Topic: s.topic, SourceKind: SourceKind, Payload: body}); err != nil {
			return err
		}
	}
	return nil
}

// issueTitle reads "name: description", the pairing PostHog's own UI shows.
// The name alone is the exception class, which collides hard on real data — a
// project with five unrelated TypeErrors gets five feed rows all reading
// "TypeError", and the row is what a user triages from. Either half alone is
// used when only one is present, and a wholly empty issue still gets a title:
// an untitled item is keyed by its UUID in the feed, which reads as noise.
func issueTitle(issue client.Issue) string {
	name := strings.TrimSpace(issue.Name)
	description := strings.TrimSpace(issue.Description)
	switch {
	case name != "" && description != "":
		return name + ": " + description
	case name != "":
		return name
	case description != "":
		return description
	}
	return "PostHog issue"
}

// issueState normalizes the status onto the payload's canonical state. An
// unrecognized status is passed through lowercased rather than mapped to
// active, so a value PostHog adds later is visible instead of silently
// counting as live.
func issueState(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "" {
		return statusActive
	}
	return normalized
}

// issueBody is the detail pane's markdown. The title already carries the
// exception and its message, so the body is the triage context that decides
// whether an issue matters: how loud it is, how long it has been running, and
// where it came from. Absent fields are omitted rather than rendered empty —
// PostHog returns aggregates only when it computed them.
func issueBody(issue client.Issue, project string) string {
	lines := make([]string, 0, 7)
	add := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			lines = append(lines, "- **"+label+"** "+value)
		}
	}
	add("Occurrences", countText(aggregate(issue.Aggregations, func(a client.Aggregations) float64 { return a.Occurrences })))
	add("Users", countText(aggregate(issue.Aggregations, func(a client.Aggregations) float64 { return a.Users })))
	add("Sessions", countText(aggregate(issue.Aggregations, func(a client.Aggregations) float64 { return a.Sessions })))
	add("First seen", issue.FirstSeen)
	add("Last seen", issue.LastSeen)
	add("Source", issue.Source)
	add("Library", issue.Library)
	add("Project", project)
	return strings.Join(lines, "\n")
}

// countText renders an aggregate as a plain integer, and "" for a
// non-positive one so an aggregate PostHog did not compute is left out rather
// than reported as a real count of zero.
func countText(v float64) string {
	if v <= 0 {
		return ""
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func aggregate(a *client.Aggregations, pick func(client.Aggregations) float64) float64 {
	if a == nil {
		return 0
	}
	return pick(*a)
}

// epochMillis parses one of PostHog's RFC 3339 timestamps. An unparseable or
// absent value yields 0, which the ingest boundary reads as "observed now"
// rather than as 1970.
func epochMillis(ts string) int64 {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}
	return parsed.UnixMilli()
}

// errorsClassifier maps an issue's status to lifecycle and names the two
// transitions worth a notification: a resolved issue that starts erroring
// again, and one that gets resolved.
type errorsClassifier struct{}

var _ models.Classifier = errorsClassifier{}

func (errorsClassifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	state := issueStateOf(current.Payload)
	lifecycle := models.LifecycleActive
	if isTerminalIssueState(state) {
		lifecycle = models.LifecycleTerminal
	}
	out := models.Classification{
		Kind:        "issue",
		Transition:  models.TransitionNone,
		Attention:   models.AttentionActivity,
		Lifecycle:   lifecycle,
		SourceState: state,
		// Keyed on when the issue was last seen, not on when it was polled, so
		// a poll that finds nothing new is not a fresh occurrence. This is what
		// keeps a steadily-firing issue from re-alerting every tick.
		OccurrenceKey: current.ExternalID + "@" + occurrenceStamp(current),
		Summary:       current.Title,
	}
	if previous == nil {
		out.Kind = "new"
		return out
	}
	switch prev := issueStateOf(previous.Payload); {
	case !isTerminalIssueState(prev) && isTerminalIssueState(state):
		out.Kind, out.Summary, out.Transition, out.ArchivedReason = state, "Resolved", models.TransitionEnteredTerminal, state
	case isTerminalIssueState(prev) && !isTerminalIssueState(state):
		out.Kind, out.Summary, out.Transition = "regressed", "Regressed", models.TransitionLeftTerminal
	case occurrenceStamp(current) != occurrenceStamp(*previous):
		out.Kind = "occurred"
	default:
		out.Kind, out.Attention = "updated", models.AttentionTrivial
	}
	return out
}

// occurrenceStamp identifies the issue's latest occurrence burst. It reads the
// raw lastSeen string rather than the parsed ObservedAt so that a timestamp
// format this connector cannot parse degrades to "no new occurrence" instead
// of to a fresh one every tick — the latter would re-notify on every poll.
func occurrenceStamp(obs models.Observation) string {
	var wire struct {
		LastSeen string `json:"lastSeen"`
	}
	if err := json.Unmarshal(obs.Payload, &wire); err == nil {
		if stamp := strings.TrimSpace(wire.LastSeen); stamp != "" {
			return stamp
		}
	}
	return strconv.FormatInt(obs.ObservedAt, 10)
}

func issueStateOf(payload []byte) string {
	_, _, state := models.CanonicalFields(payload)
	return strings.ToLower(strings.TrimSpace(state))
}

// isTerminalIssueState reports whether a status means the issue is no longer
// asking for attention. suppressed and archived are terminal for the same
// reason resolved is: the user has said they do not want to hear about it.
func isTerminalIssueState(state string) bool {
	switch state {
	case statusResolved, statusSuppressed, statusArchived:
		return true
	default:
		return false
	}
}
