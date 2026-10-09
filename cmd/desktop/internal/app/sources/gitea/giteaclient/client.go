// Package giteaclient is the desktop's Gitea/Forgejo REST client: token
// validation, the issue/PR search, the notification inbox, and the single-issue
// lookup absence confirmation needs. It owns no feed concepts — caching,
// cadence and item shaping live in cmd/desktop/internal/app/sources/gitea — and no
// persistence: the caller builds a client per tick from a freshly resolved
// host and token.
//
// HTTP plumbing — the failure taxonomy, status mapping, request logging —
// comes from sources/sourcehttp.
package giteaclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hay-kot/appkit/httpclient"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/sourcehttp"
)

const (
	sourceName = "gitea"
	apiPrefix  = "/api/v1"
)

// The involvement filters a search may ask for. Each is a boolean query
// parameter naming a relationship between the authenticated user and the item.
//
// Gitea ANDs them: requesting created and review_requested in one call returns
// items that are both, which is almost never what someone means. The caller
// issues one request per involvement and merges.
const (
	InvolvementCreated         = "created"
	InvolvementAssigned        = "assigned"
	InvolvementMentioned       = "mentioned"
	InvolvementReviewRequested = "review_requested"
	InvolvementReviewed        = "reviewed"
)

// Client is a Gitea/Forgejo API v1 client. The zero value is not usable;
// construct with NewClient.
type Client struct {
	api  *httpclient.Client
	errs sourcehttp.Errors
}

type options struct {
	logger zerolog.Logger
}

type Option func(*options)

// WithLogger enables request logging at debug level.
func WithLogger(logger zerolog.Logger) Option {
	return func(o *options) { o.logger = logger }
}

// NewClient builds a client for one instance and token. Gitea accepts a
// personal access token as a bearer credential, so sourcehttp's own auth
// middleware carries it and there is no provider-specific scheme here.
func NewClient(base, token string, opts ...Option) *Client {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return &Client{
		api: sourcehttp.New(sourcehttp.Config{
			Name:    sourceName,
			BaseURL: strings.TrimRight(base, "/"),
			Logger:  o.logger,
			Token:   func() string { return token },
		}, httpclient.Header("Accept", "application/json")),
		errs: sourcehttp.Errors{Name: sourceName},
	}
}

// User is the authenticated account. Login is the credential's account half;
// FullName and AvatarURL are what the Integrations card renders.
type User struct {
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	AvatarURL string `json:"avatar_url"`
}

// User returns the authenticated user, doubling as token validation:
// ErrUnauthorized means the token is missing, revoked, or expired.
func (c *Client) User(ctx context.Context) (User, error) {
	var user User
	if err := c.getJSON(ctx, "/user", nil, &user); err != nil {
		return User{}, err
	}
	return user, nil
}

// Version is the instance's advertised version. It answers unauthenticated, so
// connect calls it first: a URL that is not a Gitea or Forgejo instance fails
// here with "not a Gitea instance" rather than as a confusing 401 on /user.
func (c *Client) Version(ctx context.Context) (string, error) {
	var out struct {
		Version string `json:"version"`
	}
	if err := c.getJSON(ctx, "/version", nil, &out); err != nil {
		return "", err
	}
	return out.Version, nil
}

// RepoMeta is the repository an issue belongs to.
type RepoMeta struct {
	FullName string `json:"full_name"`
}

// Label is one label on an issue.
type Label struct {
	Name string `json:"name"`
}

// PullMeta is present only on pull requests. Merged is the field that makes a
// merged PR distinguishable: Issue.State is "closed" either way.
type PullMeta struct {
	Merged bool `json:"merged"`
	Draft  bool `json:"draft"`
}

// Issue is one issue or pull request. Gitea models both as an issue and marks
// the pull requests with a non-nil PullRequest.
type Issue struct {
	ID         int64     `json:"id"`
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	State      string    `json:"state"`
	HTMLURL    string    `json:"html_url"`
	UpdatedAt  time.Time `json:"updated_at"`
	User       User      `json:"user"`
	Labels     []Label   `json:"labels"`
	Repository RepoMeta  `json:"repository"`
	PullReq    *PullMeta `json:"pull_request"`
}

// IsPullRequest reports whether this item is a pull request.
func (i Issue) IsPullRequest() bool { return i.PullReq != nil }

// LifecycleState is the item's state in the vocabulary the connector stores:
// "open", "closed", or "merged". Gitea reports a merged pull request as closed
// and records the merge on the pull-request metadata, so the two are folded
// here rather than at each call site.
func (i Issue) LifecycleState() string {
	if i.PullReq != nil && i.PullReq.Merged {
		return "merged"
	}
	return strings.ToLower(i.State)
}

// SearchRequest is one issue/PR search. Every field is optional; the zero
// value searches every issue the token can see.
type SearchRequest struct {
	// Type filters to "issues" or "pulls"; empty returns both.
	Type string
	// State is "open", "closed", or "all".
	State string
	// Owner limits the search to one user's or organization's repositories.
	Owner string
	// Labels matches items carrying any of these label names.
	Labels []string
	// Text is a free-text query over title and body.
	Text string
	// Involvement names one relationship to the authenticated user
	// (InvolvementAssigned and friends); empty applies no such filter.
	Involvement string
	// Limit bounds the page; the server caps it independently.
	Limit int
}

func (r SearchRequest) params() url.Values {
	params := url.Values{}
	if r.State != "" {
		params.Set("state", r.State)
	}
	if r.Type != "" {
		params.Set("type", r.Type)
	}
	if r.Owner != "" {
		params.Set("owner", r.Owner)
	}
	if len(r.Labels) > 0 {
		params.Set("labels", strings.Join(r.Labels, ","))
	}
	if r.Text != "" {
		params.Set("q", r.Text)
	}
	if r.Involvement != "" {
		params.Set(r.Involvement, "true")
	}
	if r.Limit > 0 {
		params.Set("limit", strconv.Itoa(r.Limit))
	}
	return params
}

// SearchIssues runs one issue/PR search across every repository the token can
// see, newest-updated first — the endpoint's own default ordering, which it
// exposes no parameter to change.
//
// Unknown filter values are not an error to Gitea: it ignores them and returns
// an unfiltered page, so a typo would read as "these are all my open PRs".
// Values are validated before they get here (gitea.Config.Validate).
func (c *Client) SearchIssues(ctx context.Context, req SearchRequest) ([]Issue, error) {
	var issues []Issue
	if err := c.getJSON(ctx, "/repos/issues/search", req.params(), &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// Subject is what a notification thread is about.
type Subject struct {
	Title string `json:"title"`
	// URL is the API URL of the subject; the item number is its last segment.
	URL string `json:"url"`
	// HTMLURL links to the subject in the web UI.
	HTMLURL string `json:"html_url"`
	// Type is "Issue", "Pull", "Commit" or "Repository".
	Type string `json:"type"`
	// State is "open", "closed" or "merged" — Gitea carries the subject's
	// lifecycle on the notification, which GitHub's inbox does not.
	State string `json:"state"`
}

// Number is the issue or pull-request number, parsed from the subject's API
// URL (.../issues/<n>) because Gitea does not send it as a field. It is 0 for
// a subject that has no number, such as a commit.
func (s Subject) Number() int {
	trimmed := strings.TrimRight(s.URL, "/")
	_, numberText, ok := strings.CutLast(trimmed, "/")
	if !ok {
		return 0
	}
	number, err := strconv.Atoi(numberText)
	if err != nil || number <= 0 {
		return 0
	}
	return number
}

// Repository is the repository a notification thread belongs to.
type Repository struct {
	FullName string `json:"full_name"`
}

// NotificationThread is one entry in the notification inbox.
type NotificationThread struct {
	ID         int64      `json:"id"`
	Unread     bool       `json:"unread"`
	Pinned     bool       `json:"pinned"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Subject    Subject    `json:"subject"`
	Repository Repository `json:"repository"`
}

// Notifications lists the notification inbox, read threads included, so the
// app mirrors the full inbox and keeps triage state locally.
//
// Unlike GitHub's, this endpoint sends no ETag, Last-Modified or
// X-Poll-Interval, so there is no conditional request to make and no
// server-mandated cadence to honor: every poll is a full fetch, and the
// caller's TTL is the only thing bounding it.
func (c *Client) Notifications(ctx context.Context, limit int) ([]NotificationThread, error) {
	params := url.Values{}
	params.Set("all", "true")
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	var threads []NotificationThread
	if err := c.getJSON(ctx, "/notifications", params, &threads); err != nil {
		return nil, err
	}
	return threads, nil
}

// Issue fetches one issue or pull request by number.
//
// found is false for a 404 — the item was deleted, or the token lost access to
// its repository — which is a verdict the caller acts on rather than a failure
// that should abort a batch of lookups.
func (c *Client) Issue(ctx context.Context, owner, repo string, number int) (issue Issue, found bool, err error) {
	path := fmt.Sprintf("%s/issues/%d", repoPath(owner, repo), number)
	found, err = c.getOptional(ctx, path, nil, &issue)
	return issue, found, err
}

// getOptional is getJSON for an endpoint whose 404 means "nothing here" rather
// than a failure: the item was deleted, the token lost access to its
// repository, or the branch pair names no pull request.
func (c *Client) getOptional(ctx context.Context, path string, params url.Values, out any) (bool, error) {
	endpoint := apiPrefix + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	resp, err := c.api.Get(ctx, endpoint)
	if err != nil {
		return false, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if err := c.errs.Status(resp); err != nil {
		return false, err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return false, c.errs.Errorf("decode %s: %w", path, err)
	}
	return true, nil
}

func (c *Client) getJSON(ctx context.Context, path string, params url.Values, out any) error {
	endpoint := apiPrefix + path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	resp, err := c.api.Get(ctx, endpoint)
	if err != nil {
		return c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return c.errs.Errorf("decode %s: %w", path, err)
	}
	return nil
}
