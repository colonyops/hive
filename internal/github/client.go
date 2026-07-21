// Package github is a minimal GitHub REST client for the desktop data layer.
// It owns in-app authentication (OAuth device flow, PAT validation) and the
// typed API calls the feed needs (search, notifications). It deliberately has
// no feed concepts: profiles, unread state, and polling live in internal/feed.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIBase  = "https://api.github.com"
	defaultAuthBase = "https://github.com"
	requestTimeout  = 30 * time.Second
)

// Sentinel errors form the taxonomy the UI maps onto design states
// (unauthorized -> re-auth, rate limited / unreachable -> "GitHub unreachable").
var (
	ErrUnauthorized = errors.New("github: unauthorized")
	ErrRateLimited  = errors.New("github: rate limited")
	ErrUnreachable  = errors.New("github: unreachable")
)

// Client is a GitHub REST v3 client. The zero value is not usable; construct
// with NewClient.
type Client struct {
	httpClient *http.Client
	apiBase    string
	authBase   string
	token      string
}

type Option func(*Client)

// WithToken sets the bearer token used for API calls.
func WithToken(token string) Option {
	return func(c *Client) { c.token = token }
}

// WithAPIBase overrides the REST API base URL (tests).
func WithAPIBase(base string) Option {
	return func(c *Client) { c.apiBase = base }
}

// WithAuthBase overrides the OAuth base URL used by the device flow (tests).
func WithAuthBase(base string) Option {
	return func(c *Client) { c.authBase = base }
}

// WithHTTPClient overrides the underlying HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) { c.httpClient = httpClient }
}

func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: requestTimeout},
		apiBase:    defaultAPIBase,
		authBase:   defaultAuthBase,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithTokenCopy returns a copy of the client using the given token.
func (c *Client) WithTokenCopy(token string) *Client {
	clone := *c
	clone.token = token
	return &clone
}

// User returns the authenticated user. It doubles as token validation:
// ErrUnauthorized means the token is missing, revoked, or expired.
func (c *Client) User(ctx context.Context) (User, error) {
	var user User
	if err := c.getJSON(ctx, "/user", nil, &user); err != nil {
		return User{}, err
	}
	return user, nil
}

// SearchRequest is one search in a batched GraphQL issue/PR search.
type SearchRequest struct {
	Query string // GitHub search syntax; "sort:updated-desc" is appended by the client
	Limit int    // max items for this search (GraphQL `first`)
}

// SearchIssuesBatch runs every request as an aliased search field of a single
// GraphQL query, newest-updated first. Results map back to requests by index:
// out[i] answers reqs[i]. Query text is passed via GraphQL variables ($q0,
// $q1, ...), never interpolated.
func (c *Client) SearchIssuesBatch(ctx context.Context, reqs []SearchRequest) ([][]SearchItem, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	doc, variables := buildSearchQuery(reqs)
	var data map[string]gqlSearchResult
	if err := c.postGraphQL(ctx, doc, variables, &data); err != nil {
		return nil, err
	}

	results := make([][]SearchItem, len(reqs))
	for i := range reqs {
		nodes := data[fmt.Sprintf("s%d", i)].Nodes
		items := make([]SearchItem, 0, len(nodes))
		for _, node := range nodes {
			author := ""
			if node.Author != nil {
				author = node.Author.Login
			}
			items = append(items, SearchItem{
				Number:        node.Number,
				Title:         node.Title,
				Body:          node.Body,
				State:         node.State,
				URL:           node.URL,
				Repo:          node.Repository.NameWithOwner,
				Author:        author,
				Labels:        node.Labels.Nodes,
				IsPullRequest: node.Type == "PullRequest",
				Draft:         node.Draft,
				CreatedAt:     node.CreatedAt,
				UpdatedAt:     node.UpdatedAt,
			})
		}
		results[i] = items
	}
	return results, nil
}

// buildSearchQuery constructs the aliased GraphQL document and its variable
// map for a batch of search requests. Alias sN and variable $qN correspond to
// reqs[N]; each variable value is the request's query with
// " sort:updated-desc" appended.
func buildSearchQuery(reqs []SearchRequest) (doc string, variables map[string]any) {
	variables = make(map[string]any, len(reqs))
	var b strings.Builder
	b.WriteString("query (")
	for i := range reqs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "$q%d: String!", i)
		variables[fmt.Sprintf("q%d", i)] = reqs[i].Query + " sort:updated-desc"
	}
	b.WriteString(") {\n")
	for i, req := range reqs {
		fmt.Fprintf(&b, "  s%d: search(query: $q%d, type: ISSUE, first: %d) {\n", i, i, req.Limit)
		b.WriteString(`    nodes {
      __typename
      ... on Issue {
        number title body state url createdAt updatedAt
        author { login }
        repository { nameWithOwner }
        labels(first: 20) { nodes { name } }
      }
      ... on PullRequest {
        number title body state url isDraft createdAt updatedAt
        author { login }
        repository { nameWithOwner }
        labels(first: 20) { nodes { name } }
      }
    }
  }
`)
	}
	b.WriteString("}\n")
	return b.String(), variables
}

type gqlSearchResult struct {
	Nodes []gqlSearchNode `json:"nodes"`
}

type gqlSearchNode struct {
	Type       string              `json:"__typename"`
	Number     int                 `json:"number"`
	Title      string              `json:"title"`
	Body       string              `json:"body"`
	State      string              `json:"state"`
	URL        string              `json:"url"`
	Draft      bool                `json:"isDraft"`
	CreatedAt  time.Time           `json:"createdAt"`
	UpdatedAt  time.Time           `json:"updatedAt"`
	Author     *gqlSearchAuthor    `json:"author"`
	Repository gqlSearchRepository `json:"repository"`
	Labels     gqlSearchLabels     `json:"labels"`
}

type gqlSearchAuthor struct {
	Login string `json:"login"`
}

type gqlSearchRepository struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type gqlSearchLabels struct {
	Nodes []Label `json:"nodes"`
}

// NotificationsResult is one notifications poll. When NotModified is true the
// inbox has not changed since the ifModifiedSince timestamp and Items is nil —
// the caller keeps its cached copy. LastModified echoes the response's
// Last-Modified header for the next conditional request, and PollInterval is
// the server-mandated minimum seconds between polls (X-Poll-Interval, 0 when
// the header is absent).
type NotificationsResult struct {
	Items        []Notification
	NotModified  bool
	LastModified string
	PollInterval int
}

// Notifications lists the user's notification inbox, including read threads,
// so the app can mirror the full inbox and keep triage state locally. A
// non-empty ifModifiedSince makes the request conditional: authenticated 304
// responses are free of rate-limit cost, so polling an unchanged inbox costs
// nothing.
func (c *Client) Notifications(ctx context.Context, limit int, ifModifiedSince string) (NotificationsResult, error) {
	params := url.Values{}
	params.Set("all", "true")
	params.Set("per_page", strconv.Itoa(limit))

	var notifications []Notification
	meta, err := c.getJSONConditional(ctx, "/notifications", params, ifModifiedSince, &notifications)
	if err != nil {
		return NotificationsResult{}, err
	}
	return NotificationsResult{
		Items:        notifications,
		NotModified:  meta.notModified,
		LastModified: meta.lastModified,
		PollInterval: meta.pollInterval,
	}, nil
}

func (c *Client) getJSON(ctx context.Context, path string, params url.Values, out any) error {
	_, err := c.getJSONConditional(ctx, path, params, "", out)
	return err
}

// postGraphQL executes one GraphQL request and decodes the data object into
// out. It maps transport failures to ErrUnreachable, non-2xx statuses through
// statusError, and GraphQL RATE_LIMITED errors to ErrRateLimited.
func (c *Client) postGraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{Query: query, Variables: variables})
	if err != nil {
		return fmt.Errorf("github: encode graphql request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := statusError(resp); err != nil {
		return err
	}

	var response struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("github: decode graphql: %w", err)
	}
	if len(response.Errors) > 0 {
		messages := make([]string, 0, len(response.Errors))
		for _, gqlErr := range response.Errors {
			if gqlErr.Type == "RATE_LIMITED" {
				return fmt.Errorf("github: graphql: %w", ErrRateLimited)
			}
			if gqlErr.Message != "" {
				messages = append(messages, gqlErr.Message)
			}
		}
		if len(messages) == 0 {
			messages = append(messages, "request failed")
		}
		return fmt.Errorf("github: graphql: %s", strings.Join(messages, "; "))
	}
	if err := json.Unmarshal(response.Data, out); err != nil {
		return fmt.Errorf("github: decode graphql data: %w", err)
	}
	return nil
}

// condMeta carries the conditional-request metadata of a response.
type condMeta struct {
	notModified  bool
	lastModified string
	pollInterval int
}

// getJSONConditional performs a GET, optionally conditional on
// ifModifiedSince. A 304 response is a success with notModified set and out
// untouched; every other non-2xx status maps through statusError.
func (c *Client) getJSONConditional(ctx context.Context, path string, params url.Values, ifModifiedSince string, out any) (condMeta, error) {
	endpoint := c.apiBase + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return condMeta{}, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if ifModifiedSince != "" {
		req.Header.Set("If-Modified-Since", ifModifiedSince)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return condMeta{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	meta := condMeta{
		lastModified: resp.Header.Get("Last-Modified"),
		pollInterval: parsePollInterval(resp.Header.Get("X-Poll-Interval")),
	}
	if resp.StatusCode == http.StatusNotModified {
		meta.notModified = true
		return meta, nil
	}
	if err := statusError(resp); err != nil {
		return condMeta{}, err
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return condMeta{}, fmt.Errorf("github: decode %s: %w", path, err)
	}
	return meta, nil
}

// parsePollInterval parses the X-Poll-Interval header. A missing or
// non-numeric value reads as 0 ("no server mandate"): the header is advisory
// input, not a failure the caller could act on.
func parsePollInterval(value string) int {
	if value == "" {
		return 0
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0
	}
	return seconds
}

func statusError(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return ErrRateLimited
	case resp.StatusCode == http.StatusForbidden:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if forbiddenIsRateLimit(resp, body) {
			return ErrRateLimited
		}
		return fmt.Errorf("github: %s %s: %s", resp.Request.Method, resp.Request.URL.Path, summarize(resp.StatusCode, body))
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("github: %s %s: %s", resp.Request.Method, resp.Request.URL.Path, summarize(resp.StatusCode, body))
	}
}

// forbiddenIsRateLimit distinguishes rate-limit 403s from permission 403s.
// Primary limits set X-RateLimit-Remaining: 0; secondary (abuse) limits keep
// a nonzero remaining but send Retry-After and a "rate limit" message.
func forbiddenIsRateLimit(resp *http.Response, body []byte) bool {
	return resp.Header.Get("X-RateLimit-Remaining") == "0" ||
		resp.Header.Get("Retry-After") != "" ||
		strings.Contains(strings.ToLower(string(body)), "rate limit")
}

func summarize(status int, body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Message != "" {
		return fmt.Sprintf("HTTP %d: %s", status, payload.Message)
	}
	return fmt.Sprintf("HTTP %d", status)
}
