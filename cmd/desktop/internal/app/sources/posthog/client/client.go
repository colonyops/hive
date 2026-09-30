// Package client is the desktop's PostHog HTTP client: project listing (which
// doubles as token validation), the error-tracking issue query, and the
// insight-alert list. HTTP plumbing (failure taxonomy, rate-limit mapping,
// logging) comes from sources/sourcehttp.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hay-kot/appkit/httpclient"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

const sourceName = "posthog"

// pageSize is the one page a poll reads from a paginated list endpoint.
// Neither list is unbounded in practice — a project's alerts run to dozens at
// most, and a personal API key sees a handful of projects — so a single
// bounded page is the whole set. Where it is not, the caller reports the
// truncation rather than hiding it.
const pageSize = 100

// Project is one PostHog project. Its numeric id is what every project-scoped
// API path and UI deep link is built from.
type Project struct {
	ID   int    `json:"id"`
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// Aggregations are an issue's counts over the queried date range. PostHog
// sends them as floats, so they are kept as floats rather than rounded on the
// way in.
type Aggregations struct {
	Occurrences float64 `json:"occurrences"`
	Users       float64 `json:"users"`
	Sessions    float64 `json:"sessions"`
}

// Issue is one error-tracking issue. ID is the stable identity an inbox item
// is keyed on: PostHog groups every occurrence of one exception under it, so
// keying on the issue — never on an event — is what rolls a high-volume error
// up into a single feed item.
type Issue struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Status       string        `json:"status"`
	FirstSeen    string        `json:"first_seen"`
	LastSeen     string        `json:"last_seen"`
	Library      string        `json:"library"`
	Source       string        `json:"source"`
	Aggregations *Aggregations `json:"aggregations"`
}

// DateRange bounds an issue query. Only the lower bound is configurable — a
// feed always wants "up to now".
type DateRange struct {
	DateFrom string `json:"date_from"`
}

// IssuesRequest is the typed body of the issues query. Field names are
// PostHog's camelCase, which is the wire form that endpoint validates against.
type IssuesRequest struct {
	DateRange          DateRange `json:"dateRange"`
	Status             string    `json:"status"`
	OrderBy            string    `json:"orderBy"`
	OrderDirection     string    `json:"orderDirection"`
	Limit              int       `json:"limit"`
	FilterTestAccounts bool      `json:"filterTestAccounts"`
}

// AlertInsight is the insight an alert monitors. ShortID — not the numeric id
// — is what a PostHog insight URL is built from.
type AlertInsight struct {
	ID      int    `json:"id"`
	ShortID string `json:"short_id"`
	Name    string `json:"name"`
}

// Alert is one insight alert. Threshold and Condition are carried verbatim so
// a function node can route on them without this package modelling PostHog's
// per-insight-kind alert config shapes.
type Alert struct {
	ID                  string          `json:"id"`
	Name                string          `json:"name"`
	State               string          `json:"state"`
	Enabled             bool            `json:"enabled"`
	Insight             AlertInsight    `json:"insight"`
	Threshold           json.RawMessage `json:"threshold"`
	Condition           json.RawMessage `json:"condition"`
	CalculationInterval string          `json:"calculation_interval"`
	LastValue           *float64        `json:"last_value"`
	LastNotifiedAt      string          `json:"last_notified_at"`
	LastCheckedAt       string          `json:"last_checked_at"`
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

// NewClient builds a client for one host and token. Callers build it per tick
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

// Projects lists every project the token can see, doubling as validation:
// ErrUnauthorized means the key is missing, revoked, or lacks project:read.
// A key scoped to a subset of projects returns only that subset, which is
// exactly the list the connect flow should offer.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	resp, err := c.api.Get(ctx, "/api/projects/?limit="+strconv.Itoa(pageSize))
	if err != nil {
		return nil, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return nil, err
	}
	var envelope struct {
		Results []Project `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, c.errs.Errorf("decode projects: %w", err)
	}
	return envelope.Results, nil
}

// Issues runs the error-tracking issue query for one project.
//
// This goes through the dedicated error_tracking/query/issues endpoint rather
// than the generic /query/ one: both wrap the same internal ErrorTrackingQuery,
// but only this one is a typed, scoped, OpenAPI-documented request whose
// filters and ordering PostHog maintains. Posting a raw ErrorTrackingQuery is
// what breaks when they add a required field (PostHog/posthog#40075).
func (c *Client) Issues(ctx context.Context, projectID int, req IssuesRequest) ([]Issue, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, c.errs.Errorf("encode issues query: %w", err)
	}
	path := "/api/projects/" + strconv.Itoa(projectID) + "/error_tracking/query/issues/"

	resp, err := c.api.Post(ctx, path, bytes.NewReader(body),
		httpclient.Header("Content-Type", "application/json"))
	if err != nil {
		return nil, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return nil, err
	}
	var envelope struct {
		Results []Issue `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, c.errs.Errorf("decode issues: %w", err)
	}
	return envelope.Results, nil
}

// Alerts lists one project's insight alerts. The response carries every alert
// with its current state, firing or not — so unlike Grafana's Alertmanager
// list, an alert missing from it was deleted rather than resolved.
//
// truncated reports that the project has more alerts than one page, so the
// caller can say so instead of quietly serving a partial set.
func (c *Client) Alerts(ctx context.Context, projectID int) (alerts []Alert, truncated bool, err error) {
	path := "/api/projects/" + strconv.Itoa(projectID) + "/alerts/?limit=" + strconv.Itoa(pageSize)

	resp, err := c.api.Get(ctx, path)
	if err != nil {
		return nil, false, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return nil, false, err
	}
	var envelope struct {
		Next    string  `json:"next"`
		Results []Alert `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, false, c.errs.Errorf("decode alerts: %w", err)
	}
	return envelope.Results, envelope.Next != "", nil
}

// IssueURL is the deep link to one issue in the PostHog UI.
func IssueURL(base string, projectID int, issueID string) string {
	return fmt.Sprintf("%s/project/%d/error_tracking/%s", base, projectID, issueID)
}

// InsightURL is the deep link to the insight an alert monitors. PostHog has no
// per-alert page, so an alert item links to the insight whose threshold it
// watches; an alert whose insight carries no short id links to the project's
// alert list instead of nowhere.
func InsightURL(base string, projectID int, shortID string) string {
	if shortID == "" {
		return fmt.Sprintf("%s/project/%d/insights", base, projectID)
	}
	return fmt.Sprintf("%s/project/%d/insights/%s", base, projectID, shortID)
}
