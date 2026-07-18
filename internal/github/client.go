// Package github is a minimal GitHub REST client for the desktop data layer.
// It owns in-app authentication (OAuth device flow, PAT validation) and the
// typed API calls the feed needs (search, notifications). It deliberately has
// no feed concepts: profiles, unread state, and polling live in internal/feed.
package github

import (
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

// SearchIssues runs a GitHub issue/PR search query, newest-updated first.
func (c *Client) SearchIssues(ctx context.Context, query string, limit int) (SearchResult, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("sort", "updated")
	params.Set("order", "desc")
	params.Set("per_page", strconv.Itoa(limit))

	var result SearchResult
	if err := c.getJSON(ctx, "/search/issues", params, &result); err != nil {
		return SearchResult{}, err
	}
	return result, nil
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
