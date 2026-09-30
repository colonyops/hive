// Package client is the desktop's Grafana HTTP client: token validation, a
// PromQL query through the datasource proxy, the firing-alerts list, and the
// IRM/OnCall alert groups. HTTP plumbing (failure taxonomy, rate-limit
// mapping, logging) comes from sources/sourcehttp.
package client

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/hay-kot/appkit/httpclient"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

const sourceName = "grafana"

// Org is the organization a token authenticates against, from GET /api/org/.
// Its id is half of a stack's account identity: one stack can host several
// orgs and a token belongs to exactly one.
type Org struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// QueryResult is a Prometheus query response, carried verbatim so a function
// node can read it without the connector modelling Prometheus' value shapes.
type QueryResult struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

// Alert is one firing alert from the Alertmanager v2 API. Fingerprint is the
// stable per-instance identity an inbox item is keyed on.
type Alert struct {
	Fingerprint string            `json:"fingerprint"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	Status      struct {
		State string `json:"state"`
	} `json:"status"`
}

// AlertRuleURL is the deep link to the rule an alert instance was evaluated
// from. An instance has no page of its own — the rule's view is where its
// history, query and annotations are — so an alert whose labels carry no rule
// uid links to the alert list rather than to a broken rule page.
func AlertRuleURL(base, ruleUID string) string {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return ""
	}
	if ruleUID = strings.TrimSpace(ruleUID); ruleUID == "" {
		return base + "/alerting/list"
	}
	return base + "/alerting/grafana/" + url.PathEscape(ruleUID) + "/view"
}

type Client struct {
	api  *httpclient.Client
	errs sourcehttp.Errors
}

type options struct {
	logger zerolog.Logger
}

type Option func(*options)

func WithLogger(logger zerolog.Logger) Option {
	return func(o *options) { o.logger = logger }
}

// NewClient builds a client for one stack and token. Callers build it per tick
// from a freshly resolved token, so a rotated credential is never used stale.
func NewClient(base, token string, opts ...Option) *Client {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return &Client{
		api: sourcehttp.New(sourcehttp.Config{
			Name:    sourceName,
			BaseURL: base,
			Logger:  o.logger,
			Token:   func() string { return token },
		}, httpclient.Header("Accept", "application/json")),
		errs: sourcehttp.Errors{Name: sourceName},
	}
}

// ValidateToken reads the token's org, doubling as validation: ErrUnauthorized
// means the token is missing, revoked, or scoped to no org.
func (c *Client) ValidateToken(ctx context.Context) (Org, error) {
	resp, err := c.api.Get(ctx, "/api/org/")
	if err != nil {
		return Org{}, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return Org{}, err
	}
	var org Org
	if err := json.NewDecoder(resp.Body).Decode(&org); err != nil {
		return Org{}, c.errs.Errorf("decode org: %w", err)
	}
	return org, nil
}

// Query runs a PromQL instant query through the datasource proxy. A non-success
// status in the response body is an error even when the HTTP status was 200.
func (c *Client) Query(ctx context.Context, dsUID, promql string) (QueryResult, error) {
	path := "/api/datasources/proxy/uid/" + url.PathEscape(dsUID) + "/api/v1/query"
	form := url.Values{"query": {promql}}.Encode()

	resp, err := c.api.Post(ctx, path, strings.NewReader(form),
		httpclient.Header("Content-Type", "application/x-www-form-urlencoded"))
	if err != nil {
		return QueryResult{}, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return QueryResult{}, err
	}

	var envelope struct {
		Status    string      `json:"status"`
		Data      QueryResult `json:"data"`
		ErrorType string      `json:"errorType"`
		Error     string      `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return QueryResult{}, c.errs.Errorf("decode query response: %w", err)
	}
	if envelope.Status != "success" {
		msg := envelope.Error
		if msg == "" {
			msg = envelope.Status
		}
		return QueryResult{}, c.errs.Errorf("query failed: %s", msg)
	}
	return envelope.Data, nil
}

// Alerts lists the stack's currently firing alerts, narrowed to those matching
// every matcher. The response is the complete active set for those matchers,
// which is what lets the connector treat an absent alert as authoritatively
// resolved.
//
// Matchers are passed to Alertmanager verbatim as repeated `filter` params —
// the endpoint owns the `=`, `!=`, `=~`, `!~` syntax, so re-encoding it here
// would only be a second thing to keep in step with it.
func (c *Client) Alerts(ctx context.Context, matchers []string) ([]Alert, error) {
	path := "/api/alertmanager/grafana/api/v2/alerts"
	if len(matchers) > 0 {
		filters := url.Values{}
		for _, matcher := range matchers {
			filters.Add("filter", matcher)
		}
		path += "?" + filters.Encode()
	}

	resp, err := c.api.Get(ctx, path)
	if err != nil {
		return nil, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return nil, err
	}
	var alerts []Alert
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return nil, c.errs.Errorf("decode alerts: %w", err)
	}
	return alerts, nil
}
