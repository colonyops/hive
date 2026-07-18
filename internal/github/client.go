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

// Notifications lists the user's notification inbox, including read threads,
// so the app can mirror the full inbox and keep triage state locally.
func (c *Client) Notifications(ctx context.Context, limit int) ([]Notification, error) {
	params := url.Values{}
	params.Set("all", "true")
	params.Set("per_page", strconv.Itoa(limit))

	var notifications []Notification
	if err := c.getJSON(ctx, "/notifications", params, &notifications); err != nil {
		return nil, err
	}
	return notifications, nil
}

func (c *Client) getJSON(ctx context.Context, path string, params url.Values, out any) error {
	endpoint := c.apiBase + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
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

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("github: decode %s: %w", path, err)
	}
	return nil
}

func statusError(resp *http.Response) error {
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return ErrRateLimited
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("github: %s %s: %s", resp.Request.Method, resp.Request.URL.Path, summarize(resp.StatusCode, body))
	}
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
