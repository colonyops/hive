package client

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/hay-kot/appkit/httpclient"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

const irmSourceName = "grafana-irm"

// OnCallURL reads the OnCall API base the stack's IRM plugin is configured
// against. OnCall is not hosted on the stack — it answers on its own regional
// host — so the base cannot be derived from the stack URL and has to be asked
// for. It is a per-stack constant, so callers cache it rather than paying this
// request on every poll.
func (c *Client) OnCallURL(ctx context.Context) (string, error) {
	resp, err := c.api.Get(ctx, "/api/plugins/grafana-irm-app/settings")
	if err != nil {
		return "", c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return "", err
	}
	var settings struct {
		JSONData struct {
			OnCallAPIURL string `json:"onCallApiUrl"`
		} `json:"jsonData"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return "", c.errs.Errorf("decode irm plugin settings: %w", err)
	}
	base := strings.TrimRight(strings.TrimSpace(settings.JSONData.OnCallAPIURL), "/")
	if base == "" {
		return "", c.errs.Errorf("the IRM plugin reports no OnCall API URL; enable IRM on this stack")
	}
	return base, nil
}

// AlertGroup is one IRM alert group from the OnCall v1 API.
//
// The group, not the alert, is the unit: IRM collapses related alerts into one
// group, and the group is what reaches an on-call channel and what a responder
// acknowledges.
type AlertGroup struct {
	// ID is the stable per-group identity an inbox item is keyed on.
	ID string `json:"id"`
	// State is one of new, acknowledged, silenced, resolved.
	State          string     `json:"state"`
	Title          string     `json:"title"`
	IntegrationID  string     `json:"integration_id"`
	TeamID         string     `json:"team_id"`
	AlertsCount    int        `json:"alerts_count"`
	Permalinks     Permalinks `json:"permalinks"`
	Labels         []Label    `json:"labels"`
	LastAlert      *IRMAlert  `json:"last_alert"`
	CreatedAt      string     `json:"created_at"`
	AcknowledgedAt string     `json:"acknowledged_at"`
	SilencedAt     string     `json:"silenced_at"`
}

// IRMAlert carries the source payload of one notification in a group. Payload
// is provider-defined, so interpretation stays in the connector.
type IRMAlert struct {
	ID           string          `json:"id"`
	AlertGroupID string          `json:"alert_group_id"`
	CreatedAt    string          `json:"created_at"`
	Payload      json.RawMessage `json:"payload"`
}

// Permalinks are an alert group's links out. Every field is optional and the
// object itself may be null, which decodes to the zero value.
type Permalinks struct {
	Slack    string `json:"slack"`
	Web      string `json:"web"`
	Telegram string `json:"telegram"`
}

// Label is one IRM label. Both halves carry an id and a display name; the name
// is the form the API's `label` filter matches on.
type Label struct {
	Key   LabelPart `json:"key"`
	Value LabelPart `json:"value"`
}

type LabelPart struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LabelMap flattens an alert group's labels to the name/name pairs a function
// node routes on. A label with no key name is dropped — it cannot be addressed.
func (g AlertGroup) LabelMap() map[string]string {
	if len(g.Labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(g.Labels))
	for _, label := range g.Labels {
		if key := strings.TrimSpace(label.Key.Name); key != "" {
			out[key] = label.Value.Name
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// URL is the link a feed item points at: the Slack thread where the on-call
// conversation already is, else the IRM web page, else Telegram.
func (g AlertGroup) URL() string {
	for _, link := range []string{g.Permalinks.Slack, g.Permalinks.Web, g.Permalinks.Telegram} {
		if trimmed := strings.TrimSpace(link); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// AlertGroupQuery scopes a listing. Empty fields are not sent, so the zero
// query is the stack's whole alert-group set.
type AlertGroupQuery struct {
	// States filters on lifecycle: new, acknowledged, silenced, resolved.
	States []string
	// IntegrationID pins the listing to one IRM integration — the unit a
	// squad's upstream routes hang off.
	IntegrationID string
	// TeamID pins the listing to one IRM team.
	TeamID string
}

const (
	alertGroupPageSize = 100
	// maxAlertGroupPages bounds paging. A snapshot the connector treats as
	// authoritative has to be complete: a truncated page walk would read as
	// "these groups are gone" and archive live alerts, so running out of pages
	// fails the poll instead of returning a short set.
	maxAlertGroupPages = 20
)

// OnCallClient talks to the OnCall v1 API. It is separate from Client because
// OnCall answers on its own host: same stack, same token, different base URL.
type OnCallClient struct {
	api  *httpclient.Client
	errs sourcehttp.Errors
}

// NewOnCallClient builds a client for one stack's OnCall API. The stack URL
// rides along in X-Grafana-URL, which is how OnCall resolves a Grafana service
// account token to the org it belongs to.
func NewOnCallClient(oncallBase, stackURL, token string, opts ...Option) *OnCallClient {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return &OnCallClient{
		api: sourcehttp.New(sourcehttp.Config{
			Name:    irmSourceName,
			BaseURL: oncallBase,
			Logger:  o.logger,
			Token:   func() string { return token },
		},
			httpclient.Header("Accept", "application/json"),
			httpclient.Header("X-Grafana-URL", stackURL),
		),
		errs: sourcehttp.Errors{Name: irmSourceName},
	}
}

// AlertGroups lists every alert group matching the query, fetching and paging
// each state separately because the OnCall API accepts only one state per
// request. A later state replaces an earlier duplicate while retaining its
// position in the combined result.
func (c *OnCallClient) AlertGroups(ctx context.Context, query AlertGroupQuery) ([]AlertGroup, error) {
	states := query.States
	if len(states) == 0 {
		states = []string{""}
	}

	var groups []AlertGroup
	positions := make(map[string]int)
	for _, state := range states {
		stateGroups, err := c.alertGroupsForState(ctx, query, state)
		if err != nil {
			return nil, err
		}
		for _, group := range stateGroups {
			if position, ok := positions[group.ID]; ok {
				groups[position] = group
				continue
			}
			positions[group.ID] = len(groups)
			groups = append(groups, group)
		}
	}
	return groups, nil
}

func (c *OnCallClient) alertGroupsForState(ctx context.Context, query AlertGroupQuery, state string) ([]AlertGroup, error) {
	var groups []AlertGroup
	for page := 1; page <= maxAlertGroupPages; page++ {
		body, err := c.alertGroupPage(ctx, query, state, page)
		if err != nil {
			return nil, err
		}
		groups = append(groups, body.Results...)
		if body.Next == nil || strings.TrimSpace(*body.Next) == "" {
			return groups, nil
		}
	}
	return nil, c.errs.Errorf("more than %d alert groups matched; scope this source to an integration or team",
		maxAlertGroupPages*alertGroupPageSize)
}

type alertGroupPage struct {
	Results []AlertGroup `json:"results"`
	// Next is null on the last page. Only its presence is read — the walk pages
	// by number rather than following the URL, so the client never has to
	// re-authenticate against a host the response chose.
	Next *string `json:"next"`
}

func (c *OnCallClient) alertGroupPage(ctx context.Context, query AlertGroupQuery, state string, page int) (alertGroupPage, error) {
	params := url.Values{}
	if state != "" {
		params.Set("state", state)
	}
	if query.IntegrationID != "" {
		params.Set("integration_id", query.IntegrationID)
	}
	if query.TeamID != "" {
		params.Set("team_id", query.TeamID)
	}
	params.Set("perpage", strconv.Itoa(alertGroupPageSize))
	params.Set("page", strconv.Itoa(page))

	resp, err := c.api.Get(ctx, "/api/v1/alert_groups/?"+params.Encode())
	if err != nil {
		return alertGroupPage{}, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return alertGroupPage{}, err
	}
	var body alertGroupPage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return alertGroupPage{}, c.errs.Errorf("decode alert groups: %w", err)
	}
	return body, nil
}
