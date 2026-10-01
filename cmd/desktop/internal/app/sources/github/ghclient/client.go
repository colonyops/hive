// Package ghclient is the desktop's own GitHub REST/GraphQL client: in-app
// authentication (OAuth device flow, PAT validation via User) and the typed
// API calls the connector needs (batched search, notifications, single-issue
// hydration). It owns no feed concepts — caching, polling cadence, and
// unread state live in cmd/desktop/internal/app/sources/github/feed — and no
// persistence: callers hold whatever token they have and pass it in via
// WithTokenCopy.
//
// HTTP plumbing — the failure taxonomy, status mapping, conditional requests,
// request logging — comes from sources/sourcehttp.
package ghclient

import (
	"bytes"
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
	defaultAPIBase  = "https://api.github.com"
	defaultAuthBase = "https://github.com"
	apiVersion      = "2022-11-28"
	sourceName      = "github"
)

// Client is a GitHub REST v3 / GraphQL client. The zero value is not usable;
// construct with NewClient.
type Client struct {
	api  *httpclient.Client
	auth *httpclient.Client
	errs sourcehttp.Errors
	// token is read per request through bearer, so WithTokenCopy stays a
	// plain struct copy that shares api/auth and their connection pools.
	token string
}

type options struct {
	apiBase  string
	authBase string
	logger   zerolog.Logger
}

type Option func(*options)

// WithAPIBase overrides the REST API base URL (tests, cmd/desktop/devserver).
func WithAPIBase(base string) Option {
	return func(o *options) { o.apiBase = base }
}

// WithAuthBase overrides the OAuth base URL used by the device flow (tests).
func WithAuthBase(base string) Option {
	return func(o *options) { o.authBase = base }
}

// WithLogger enables request logging at debug level.
func WithLogger(logger zerolog.Logger) Option {
	return func(o *options) { o.logger = logger }
}

func NewClient(opts ...Option) *Client {
	o := options{apiBase: defaultAPIBase, authBase: defaultAuthBase}
	for _, opt := range opts {
		opt(&o)
	}

	return &Client{
		api: sourcehttp.New(sourcehttp.Config{
			Name:    sourceName,
			BaseURL: o.apiBase,
			Logger:  o.logger,
		},
			httpclient.Header("Accept", "application/vnd.github+json"),
			httpclient.Header("X-GitHub-Api-Version", apiVersion),
			httpclient.JSONContent(),
		),
		auth: sourcehttp.New(sourcehttp.Config{
			Name:    sourceName + "-auth",
			BaseURL: o.authBase,
			Logger:  o.logger,
		},
			httpclient.Header("Accept", "application/json"),
		),
		errs: sourcehttp.Errors{Name: sourceName, Forbidden: forbiddenIsRateLimit},
	}
}

// bearer resolves the token off the receiver, so a clone authenticates as
// itself rather than as the template it was copied from.
func (c *Client) bearer() httpclient.Middleware {
	return httpclient.BearerAuth(func() string { return c.token })
}

// WithTokenCopy returns a copy of the client using the given token. Every
// request-scoped call clones through here rather than mutating a shared
// client, which is what lets one Client template safely back every
// connected account: multi-account fetches never race each other's token.
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

const (
	searchPageSize          = 25
	searchAliasesPerRequest = 2
)

type searchPageRequest struct {
	query       string
	limit       int
	resultIndex int
	after       string
}

// SearchIssuesBatch runs requests as aliased search fields, newest-updated
// first. GitHub times out when statusCheckRollup is resolved for large search
// batches, so each request carries at most two aliases with pages of 25.
// Results map back by request index: out[i] answers reqs[i].
func (c *Client) SearchIssuesBatch(ctx context.Context, reqs []SearchRequest) ([][]SearchItem, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	results := make([][]SearchItem, len(reqs))
	pages := make([]searchPageRequest, 0, len(reqs))
	for i, req := range reqs {
		pages = append(pages, searchPageRequest{
			query:       req.Query,
			limit:       min(req.Limit, searchPageSize),
			resultIndex: i,
		})
	}

	for len(pages) > 0 {
		next := make([]searchPageRequest, 0, len(pages))
		for start := 0; start < len(pages); start += searchAliasesPerRequest {
			batch := pages[start:min(start+searchAliasesPerRequest, len(pages))]
			doc, variables := buildSearchPageQuery(batch)
			var data map[string]gqlSearchResult
			if err := c.postGraphQL(ctx, doc, variables, &data, false); err != nil {
				return nil, err
			}

			for i, pageRequest := range batch {
				page := data[fmt.Sprintf("s%d", i)]
				resultIndex := pageRequest.resultIndex
				results[resultIndex] = append(results[resultIndex], searchItems(page.Nodes)...)
				remaining := reqs[resultIndex].Limit - len(results[resultIndex])
				if remaining > 0 && page.PageInfo.HasNextPage {
					next = append(next, searchPageRequest{
						query:       reqs[resultIndex].Query,
						limit:       min(remaining, searchPageSize),
						resultIndex: resultIndex,
						after:       page.PageInfo.EndCursor,
					})
				}
			}
		}
		pages = next
	}
	return results, nil
}

func searchItems(nodes []gqlSearchNode) []SearchItem {
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
			Review:        reviewState(node.Draft, node.ReviewDecision),
			Checks:        checkState(node.rollupState()),
			Additions:     node.Additions,
			Deletions:     node.Deletions,
			CreatedAt:     node.CreatedAt,
			UpdatedAt:     node.UpdatedAt,
		})
	}
	return items
}

func buildSearchQuery(reqs []SearchRequest) (doc string, variables map[string]any) {
	pages := make([]searchPageRequest, len(reqs))
	for i, req := range reqs {
		pages[i] = searchPageRequest{query: req.Query, limit: min(req.Limit, searchPageSize), resultIndex: i}
	}
	return buildSearchPageQuery(pages)
}

// buildSearchPageQuery constructs one aliased GraphQL page. Query text and
// cursors use variables and are never interpolated into the document.
func buildSearchPageQuery(reqs []searchPageRequest) (doc string, variables map[string]any) {
	variables = make(map[string]any, len(reqs)*2)
	var b strings.Builder
	b.WriteString("query (")
	for i := range reqs {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "$q%d: String!, $after%d: String", i, i)
		variables[fmt.Sprintf("q%d", i)] = reqs[i].query + " sort:updated-desc"
		if reqs[i].after == "" {
			variables[fmt.Sprintf("after%d", i)] = nil
		} else {
			variables[fmt.Sprintf("after%d", i)] = reqs[i].after
		}
	}
	b.WriteString(") {\n")
	for i, req := range reqs {
		fmt.Fprintf(&b, "  s%d: search(query: $q%d, type: ISSUE, first: %d, after: $after%d) {\n", i, i, req.limit, i)
		b.WriteString(`    pageInfo { endCursor hasNextPage }
    nodes {
      __typename
      ... on Issue {
        number title body state url createdAt updatedAt
        author { login }
        repository { nameWithOwner }
        labels(first: 20) { nodes { name } }
      }
      ... on PullRequest {
        number title body state url isDraft reviewDecision additions deletions createdAt updatedAt
        statusCheckRollup { state }
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
	Nodes    []gqlSearchNode `json:"nodes"`
	PageInfo gqlPageInfo     `json:"pageInfo"`
}

type gqlPageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}

type gqlSearchNode struct {
	Type              string                `json:"__typename"`
	Number            int                   `json:"number"`
	Title             string                `json:"title"`
	Body              string                `json:"body"`
	State             string                `json:"state"`
	URL               string                `json:"url"`
	Draft             bool                  `json:"isDraft"`
	ReviewDecision    string                `json:"reviewDecision"`
	Additions         int                   `json:"additions"`
	Deletions         int                   `json:"deletions"`
	StatusCheckRollup *gqlStatusCheckRollup `json:"statusCheckRollup"`
	CreatedAt         time.Time             `json:"createdAt"`
	UpdatedAt         time.Time             `json:"updatedAt"`
	Author            *gqlSearchAuthor      `json:"author"`
	Repository        gqlRepository         `json:"repository"`
	Labels            gqlSearchLabels       `json:"labels"`
}

func (n gqlSearchNode) rollupState() string {
	if n.StatusCheckRollup == nil {
		return ""
	}
	return n.StatusCheckRollup.State
}

type gqlStatusCheckRollup struct {
	State string `json:"state"`
}

type gqlSearchAuthor struct {
	Login string `json:"login"`
}

type gqlRepository struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type gqlSearchLabels struct {
	Nodes []Label `json:"nodes"`
}

// NotificationsResult is one notifications poll. When NotModified is true the
// inbox has not changed and Items is nil — the caller keeps its cached copy.
// Validators are echoed back on the next request, and PollInterval is the
// server-mandated minimum seconds between polls (X-Poll-Interval, 0 when the
// header is absent).
type NotificationsResult struct {
	Items        []Notification
	NotModified  bool
	Validators   sourcehttp.Validators
	PollInterval int
}

// Notifications lists the user's notification inbox, including read threads,
// so the app can mirror the full inbox and keep triage state locally.
// Non-empty prev validators make the request conditional: authenticated 304
// responses are free of rate-limit cost, so polling an unchanged inbox costs
// nothing.
func (c *Client) Notifications(ctx context.Context, limit int, prev sourcehttp.Validators) (NotificationsResult, error) {
	params := url.Values{}
	params.Set("all", "true")
	params.Set("per_page", strconv.Itoa(limit))

	var notifications []Notification
	meta, err := c.getJSONConditional(ctx, "/notifications", params, prev, &notifications)
	if err != nil {
		return NotificationsResult{}, err
	}
	return NotificationsResult{
		Items:        notifications,
		NotModified:  meta.notModified,
		Validators:   meta.validators,
		PollInterval: meta.pollInterval,
	}, nil
}

func (c *Client) getJSON(ctx context.Context, path string, params url.Values, out any) error {
	_, err := c.getJSONConditional(ctx, path, params, sourcehttp.Validators{}, out)
	return err
}

// postGraphQL executes one GraphQL request and decodes the data object into
// out. GraphQL reports rate limits in the response body rather than the
// status, so RATE_LIMITED is mapped here rather than by sourcehttp.
//
// When tolerateNotFound is set, a NOT_FOUND error is not fatal: GitHub returns
// it alongside partial data (the unresolved alias is null) for a deleted or
// private repository, so out is still decoded and the caller reads the null
// alias as absent. RATE_LIMITED and every other error type remain fatal.
func (c *Client) postGraphQL(ctx context.Context, query string, variables map[string]any, out any, tolerateNotFound bool) error {
	payload, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{Query: query, Variables: variables})
	if err != nil {
		return c.errs.Errorf("encode graphql request: %w", err)
	}

	resp, err := c.api.Post(ctx, "/graphql", bytes.NewReader(payload), c.bearer())
	if err != nil {
		return c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	if err := c.errs.Status(resp); err != nil {
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
		return c.errs.Errorf("decode graphql: %w", err)
	}
	if len(response.Errors) > 0 {
		messages := make([]string, 0, len(response.Errors))
		tolerated := 0
		for _, gqlErr := range response.Errors {
			if gqlErr.Type == "RATE_LIMITED" {
				return sourcehttp.RateLimit(resp.Header)
			}
			if tolerateNotFound && gqlErr.Type == "NOT_FOUND" {
				tolerated++
				continue
			}
			if gqlErr.Message != "" {
				messages = append(messages, gqlErr.Message)
			}
		}
		// Fail unless every error was a tolerated NOT_FOUND; otherwise fall
		// through to decode the partial data those nulls sit beside.
		if tolerated < len(response.Errors) {
			if len(messages) == 0 {
				messages = append(messages, "request failed")
			}
			return c.errs.Errorf("graphql: %s", strings.Join(messages, "; "))
		}
	}
	if len(response.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Data, out); err != nil {
		return c.errs.Errorf("decode graphql data: %w", err)
	}
	return nil
}

// condMeta carries the conditional-request metadata of a response.
type condMeta struct {
	notModified  bool
	validators   sourcehttp.Validators
	pollInterval int
}

// getJSONConditional performs a GET, conditional when prev is non-empty. A 304
// is a success with notModified set and out untouched.
func (c *Client) getJSONConditional(ctx context.Context, path string, params url.Values, prev sourcehttp.Validators, out any) (condMeta, error) {
	endpoint := path
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	resp, err := c.api.Get(ctx, endpoint, c.bearer(), sourcehttp.Conditional(prev))
	if err != nil {
		return condMeta{}, c.errs.Unreachable(err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body close

	meta := condMeta{
		validators:   sourcehttp.ReadValidators(resp.Header),
		pollInterval: parsePollInterval(resp.Header.Get("X-Poll-Interval")),
	}
	if sourcehttp.NotModified(resp) {
		meta.notModified = true
		return meta, nil
	}
	if err := c.errs.Status(resp); err != nil {
		return condMeta{}, err
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return condMeta{}, c.errs.Errorf("decode %s: %w", path, err)
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

// forbiddenIsRateLimit distinguishes rate-limit 403s from permission 403s.
// Primary limits set X-RateLimit-Remaining: 0; secondary (abuse) limits keep
// a nonzero remaining but send Retry-After and a "rate limit" message.
func forbiddenIsRateLimit(resp *http.Response, body []byte) bool {
	return resp.Header.Get("X-RateLimit-Remaining") == "0" ||
		resp.Header.Get("Retry-After") != "" ||
		strings.Contains(strings.ToLower(string(body)), "rate limit")
}
