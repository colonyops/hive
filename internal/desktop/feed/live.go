package feed

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"

	"github.com/colonyops/hive/internal/github"
)

// ErrNotAuthenticated is returned when no GitHub token is available.
var ErrNotAuthenticated = errors.New("feed: not authenticated")

const (
	// notificationsMinPoll is the floor for the notifications poll interval.
	// GitHub's X-Poll-Interval header is a contract (typically 60s); it is
	// honored even when absent.
	notificationsMinPoll = 60 * time.Second
	// DefaultPollInterval is how often the pipeline producer ticks; matches
	// the settings design's default ("Every 60s").
	DefaultPollInterval = time.Minute
)

// LiveProvider fetches source items from the GitHub API with per-source
// caching and singleflight coalescing. It is the seam the desktop pipeline
// producer uses to turn a github-source node into event_log rows — the only
// GitHub-fetch implementation in the desktop.
type LiveProvider struct {
	client *github.Client
	tokens github.TokenStore
	logger zerolog.Logger
	now    func() time.Time

	mu             sync.Mutex
	cache          map[string]*cachedSource
	searchTTL      time.Duration
	searchFailures map[string]searchFailure
	flight         singleflight.Group
}

// searchFailure is one failed fetch attempt for a search cache key.
type searchFailure struct {
	at  time.Time
	err error
}

// cachedSource is one source's cached fetch. Entries are immutable once
// published: readers use their fields after releasing p.mu, so an update
// publishes a new entry rather than mutating one in place.
type cachedSource struct {
	items        []liveItem
	fetchedAt    time.Time
	lastModified string
	pollInterval time.Duration
}

// liveItem pairs the wire item with the timestamp its age derives from.
type liveItem struct {
	item      Item
	updatedAt time.Time
}

// sourceKey is the canonical cache key: two SourceDefs requesting the same
// data share one cache entry and one API request.
func sourceKey(src SourceDef) string {
	return src.Kind + "\x00" + src.Query + "\x00" + strconv.Itoa(src.effectiveLimit())
}

func NewLiveProvider(client *github.Client, tokens github.TokenStore, logger zerolog.Logger) *LiveProvider {
	return &LiveProvider{
		client:         client,
		tokens:         tokens,
		logger:         logger,
		now:            time.Now,
		cache:          make(map[string]*cachedSource),
		searchTTL:      DefaultPollInterval,
		searchFailures: make(map[string]searchFailure),
	}
}

// SetSearchTTL sets the search cache TTL. Values <= 0 retain the default
// poll interval. It is intended to be set while wiring the producer.
func (p *LiveProvider) SetSearchTTL(ttl time.Duration) {
	if ttl <= 0 {
		ttl = DefaultPollInterval
	}
	p.mu.Lock()
	p.searchTTL = ttl
	p.mu.Unlock()
}

// SourceItems returns one source's current items — served from cache within
// the fetch TTL, otherwise fetched via the conditional, singleflight-coalesced
// path (see sourceItems/fetchSource).
func (p *LiveProvider) SourceItems(ctx context.Context, src SourceDef) ([]Item, error) {
	items, err := p.sourceItems(ctx, src)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(items))
	for _, li := range items {
		out = append(out, li.item)
	}
	return out, nil
}

// Invalidate drops the fetch cache so the next call refetches. Auth changes
// call it: a different account must never be served the previous token's data.
func (p *LiveProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*cachedSource)
	p.searchFailures = make(map[string]searchFailure)
}

// notificationsTTL is the effective minimum interval between notification
// fetches for a cache entry.
func notificationsTTL(cached *cachedSource) time.Duration {
	if cached.pollInterval > notificationsMinPoll {
		return cached.pollInterval
	}
	return notificationsMinPoll
}

// sourceItems returns the source's items from cache within its TTL, otherwise
// fetching it through the conditional, singleflight-coalesced path.
func (p *LiveProvider) sourceItems(ctx context.Context, src SourceDef) ([]liveItem, error) {
	key := sourceKey(src)

	p.mu.Lock()
	cached, ok := p.cache[key]
	searchTTL := p.searchTTL
	failure, failed := p.searchFailures[key]
	p.mu.Unlock()
	if ok {
		ttl := searchTTL
		if src.Kind == "notifications" {
			ttl = notificationsTTL(cached)
		}
		if p.now().Sub(cached.fetchedAt) < ttl {
			return cached.items, nil
		}
	}

	if src.Kind == "search" && failed && p.now().Sub(failure.at) < searchTTL {
		return p.serveFetchError(src, cached, ok, failure.err)
	}

	items, err := p.fetchSource(ctx, src)
	if err != nil {
		return p.serveFetchError(src, cached, ok, err)
	}
	return items, nil
}

// serveFetchError preserves stale search and notifications data through
// transient failures, while authentication failures must reach the caller so
// it can prompt for a reconnect.
func (p *LiveProvider) serveFetchError(src SourceDef, cached *cachedSource, ok bool, err error) ([]liveItem, error) {
	if ok && !errors.Is(err, github.ErrUnauthorized) && !errors.Is(err, ErrNotAuthenticated) {
		p.logger.Debug().Err(err).Str("source", src.ID).Msg("source fetch failed; serving stale cache")
		return cached.items, nil
	}
	return nil, err
}

// PrefetchSearch batch-fetches search defs in one GraphQL request and stores
// their results in the SourceItems cache. A failed batch is retained per key
// so draining the individual sources does not retry it during this tick.
func (p *LiveProvider) PrefetchSearch(ctx context.Context, defs []SourceDef) error {
	representatives := make(map[string]SourceDef)
	keys := make([]string, 0, len(defs))
	for _, def := range defs {
		if def.Kind != "search" {
			continue
		}
		key := sourceKey(def)
		if _, seen := representatives[key]; seen {
			continue
		}
		representatives[key] = def
		keys = append(keys, key)
	}

	p.mu.Lock()
	ttl := p.searchTTL
	due := make([]SourceDef, 0, len(keys))
	dueKeys := make([]string, 0, len(keys))
	now := p.now()
	for _, key := range keys {
		cached := p.cache[key]
		if cached != nil && now.Sub(cached.fetchedAt) < ttl/2 {
			continue
		}
		due = append(due, representatives[key])
		dueKeys = append(dueKeys, key)
	}
	p.mu.Unlock()
	if len(due) == 0 {
		return nil
	}

	token, err := p.tokens.Token()
	if err == nil && token == "" {
		err = ErrNotAuthenticated
	}
	if err != nil {
		p.recordSearchFailures(dueKeys, err)
		return err
	}

	reqs := make([]github.SearchRequest, len(due))
	for i, def := range due {
		reqs[i] = github.SearchRequest{Query: def.Query, Limit: def.effectiveLimit()}
	}
	results, err := p.client.WithTokenCopy(token).SearchIssuesBatch(ctx, reqs)
	if err != nil {
		p.recordSearchFailures(dueKeys, err)
		return err
	}

	fetchedAt := p.now()
	p.mu.Lock()
	for i, key := range dueKeys {
		p.cache[key] = &cachedSource{items: p.searchItems(results[i]), fetchedAt: fetchedAt}
		delete(p.searchFailures, key)
	}
	p.mu.Unlock()
	return nil
}

func (p *LiveProvider) recordSearchFailures(keys []string, err error) {
	failure := searchFailure{at: p.now(), err: err}
	p.mu.Lock()
	for _, key := range keys {
		p.searchFailures[key] = failure
	}
	p.mu.Unlock()
}

// fetchSource performs the source's API request and updates the cache,
// coalescing concurrent fetches of the same source into one request.
func (p *LiveProvider) fetchSource(ctx context.Context, src SourceDef) ([]liveItem, error) {
	result, err, _ := p.flight.Do(sourceKey(src), func() (any, error) {
		return p.fetchSourceDirect(ctx, src)
	})
	if err != nil {
		return nil, err
	}
	return result.([]liveItem), nil
}

// fetchSourceDirect is the uncoalesced fetch behind fetchSource. Notifications
// requests are conditional: a 304 keeps the cached items and refreshes their
// fetch time at no rate-limit cost.
func (p *LiveProvider) fetchSourceDirect(ctx context.Context, src SourceDef) ([]liveItem, error) {
	token, err := p.tokens.Token()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrNotAuthenticated
	}
	client := p.client.WithTokenCopy(token)
	key := sourceKey(src)

	switch src.Kind {
	case "notifications":
		p.mu.Lock()
		prev := p.cache[key]
		ifModifiedSince := ""
		if prev != nil {
			ifModifiedSince = prev.lastModified
		}
		p.mu.Unlock()

		result, err := client.Notifications(ctx, src.effectiveLimit(), ifModifiedSince)
		if err != nil {
			return nil, err
		}
		pollInterval := time.Duration(result.PollInterval) * time.Second
		if result.NotModified && prev != nil {
			next := *prev
			next.fetchedAt = p.now()
			if pollInterval > 0 {
				next.pollInterval = pollInterval
			}
			if result.LastModified != "" {
				next.lastModified = result.LastModified
			}
			p.setCache(key, &next)
			return next.items, nil
		}
		items := p.notificationItems(result.Items)
		p.setCache(key, &cachedSource{
			items:        items,
			fetchedAt:    p.now(),
			lastModified: result.LastModified,
			pollInterval: pollInterval,
		})
		return items, nil
	case "search":
		results, err := client.SearchIssuesBatch(ctx, []github.SearchRequest{{
			Query: src.Query,
			Limit: src.effectiveLimit(),
		}})
		if err != nil {
			return nil, err
		}
		items := p.searchItems(results[0])
		p.setCache(key, &cachedSource{items: items, fetchedAt: p.now()})
		p.clearSearchFailure(key)
		return items, nil
	default:
		return nil, fmt.Errorf("feed: unknown source kind %q", src.Kind)
	}
}

func (p *LiveProvider) setCache(key string, cached *cachedSource) {
	p.mu.Lock()
	p.cache[key] = cached
	p.mu.Unlock()
}

func (p *LiveProvider) clearSearchFailure(key string) {
	p.mu.Lock()
	delete(p.searchFailures, key)
	p.mu.Unlock()
}

func (p *LiveProvider) searchItems(items []github.SearchItem) []liveItem {
	out := make([]liveItem, 0, len(items))
	for _, si := range items {
		kind := "Issue"
		if si.IsPullRequest {
			kind = "PR"
		}
		repo := si.Repo
		item := Item{
			ID:     itemID(repo, si.Number),
			Kind:   kind,
			Repo:   repo,
			Num:    si.Number,
			Title:  si.Title,
			Author: si.Author,
			Age:    shortAge(p.now().Sub(si.UpdatedAt)),
			Unread: true, // inbox model: unread until read (feed_item.unread)
			Labels: labelNames(si.Labels),
			Branch: suggestedBranch(kind, si.Number, si.Title),
			Body:   si.Body,
			Prompt: suggestedPrompt(kind, repo, si.Number, si.Title),
			URL:    si.URL,
		}
		out = append(out, liveItem{item: item, updatedAt: si.UpdatedAt})
	}
	return out
}

func (p *LiveProvider) notificationItems(notifications []github.Notification) []liveItem {
	out := make([]liveItem, 0, len(notifications))
	for _, n := range notifications {
		kind, ok := notificationKind(n.Subject.Type)
		if !ok {
			// Releases, discussions, CI activity: out of scope for a PR/issue
			// feed until the UI has a kind for them.
			continue
		}
		num := n.Subject.Number()
		repo := n.Repository.FullName
		id := itemID(repo, num)
		if num == 0 {
			id = "notif-" + n.ID
		}
		item := Item{
			ID:     id,
			Kind:   kind,
			Repo:   repo,
			Num:    num,
			Title:  n.Subject.Title,
			Age:    shortAge(p.now().Sub(n.UpdatedAt)),
			Unread: n.Unread,
			Reason: n.Reason,
			Branch: suggestedBranch(kind, num, n.Subject.Title),
			Body:   fmt.Sprintf("GitHub notification for %s in %s.", strings.ToLower(kind), repo),
			Prompt: suggestedPrompt(kind, repo, num, n.Subject.Title),
			URL:    htmlURLForSubject(repo, kind, num),
		}
		out = append(out, liveItem{item: item, updatedAt: n.UpdatedAt})
	}
	return out
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func notificationKind(subjectType string) (string, bool) {
	switch subjectType {
	case "PullRequest":
		return "PR", true
	case "Issue":
		return "Issue", true
	default:
		return "", false
	}
}

func itemID(repo string, num int) string {
	return fmt.Sprintf("%s#%d", repo, num)
}

func htmlURLForSubject(repo, kind string, num int) string {
	if repo == "" || num == 0 {
		return ""
	}
	segment := "issues"
	if kind == "PR" {
		segment = "pull"
	}
	return fmt.Sprintf("https://github.com/%s/%s/%d", repo, segment, num)
}

func labelNames(labels []github.Label) []string {
	if len(labels) == 0 {
		return nil
	}
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		names = append(names, label.Name)
	}
	return names
}

// suggestedBranch proposes a session branch name for acting on the item.
func suggestedBranch(kind string, num int, title string) string {
	prefix := "feat"
	if kind == "PR" {
		prefix = "review"
	}
	return fmt.Sprintf("%s/%d-%s", prefix, num, slugify(title))
}

func suggestedPrompt(kind, repo string, num int, title string) string {
	if kind == "PR" {
		return fmt.Sprintf("Review PR #%d in %s: %s. Read the diff, evaluate the approach, and summarize risks.", num, repo, title)
	}
	return fmt.Sprintf("Investigate issue #%d in %s: %s. Research the code paths involved and propose an implementation.", num, repo, title)
}

const slugMaxLen = 40

func slugify(s string) string {
	var b strings.Builder
	lastDash := true // suppress leading dash
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= slugMaxLen {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

func shortAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
}
