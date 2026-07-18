package feed

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/singleflight"

	"github.com/colonyops/hive/internal/github"
)

// ErrNotAuthenticated is returned when no GitHub token is available. The UI
// never hits it in normal flow (the feed is gated behind auth), but the
// taxonomy keeps the failure explicit.
var ErrNotAuthenticated = errors.New("feed: not authenticated")

const (
	// liveCacheTTL keeps feed switches snappy between poller refreshes
	// without hammering the search API.
	liveCacheTTL = 30 * time.Second
	// notificationsMinPoll is the floor for the notifications poll interval.
	// GitHub's X-Poll-Interval header is a contract (typically 60s); it is
	// honored even when absent.
	notificationsMinPoll = 60 * time.Second
)

// LiveProvider serves feed data from the GitHub API, with app-local unread
// state. Fetches are cached per source — keyed by what is requested, not by
// which feed or profile asked — so any number of feeds sharing a source cost
// one request per poll.
type LiveProvider struct {
	client *github.Client
	tokens github.TokenStore
	store  *Store
	logger zerolog.Logger
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]*cachedSource
	// flight coalesces concurrent fetches of the same source (by canonical
	// key) into one API request.
	flight singleflight.Group
}

// cachedSource is one source's cached fetch. Entries are immutable once
// published into the cache: readers use their fields after releasing p.mu,
// so an update must publish a new entry, never mutate one in place.
type cachedSource struct {
	items     []liveItem
	fetchedAt time.Time
	// lastModified and pollInterval drive the notifications conditional-GET
	// loop; both stay zero for search sources.
	lastModified string
	pollInterval time.Duration
}

// liveItem pairs the wire item with the timestamp unread state derives from.
type liveItem struct {
	item      Item
	updatedAt time.Time
}

// sourceKey is the canonical cache key of a source: two source definitions
// requesting the same data share one cache entry and one API request.
func sourceKey(src SourceDef) string {
	return src.Kind + "\x00" + src.Query + "\x00" + strconv.Itoa(src.effectiveLimit())
}

func NewLiveProvider(client *github.Client, tokens github.TokenStore, store *Store, logger zerolog.Logger) *LiveProvider {
	return &LiveProvider{
		client: client,
		tokens: tokens,
		store:  store,
		logger: logger,
		now:    time.Now,
		cache:  make(map[string]*cachedSource),
	}
}

func (p *LiveProvider) Profiles(ctx context.Context) ([]Profile, error) {
	defs, err := p.store.Profiles()
	if err != nil {
		return nil, err
	}
	srcByID, err := p.sourcesByID()
	if err != nil {
		return nil, err
	}

	profiles := make([]Profile, 0, len(defs))
	for _, def := range defs {
		profiles = append(profiles, p.materializeProfile(ctx, srcByID, def))
	}
	return profiles, nil
}

// CreateProfile persists a new profile with default feeds and returns it
// materialized (fetching its sources).
func (p *LiveProvider) CreateProfile(ctx context.Context, name string) (Profile, error) {
	def, err := p.store.CreateProfile(name)
	if err != nil {
		return Profile{}, err
	}
	srcByID, err := p.sourcesByID()
	if err != nil {
		return Profile{}, err
	}
	return p.materializeProfile(ctx, srcByID, def), nil
}

// Sources returns the source definitions, for the source picker in the feed
// editor.
func (p *LiveProvider) Sources(context.Context) ([]SourceDef, error) {
	return p.store.Sources()
}

// FeedDefFor returns one feed's definition, for edit prefill.
func (p *LiveProvider) FeedDefFor(_ context.Context, profileID, feedID string) (FeedDef, error) {
	return p.store.FeedDefFor(profileID, feedID)
}

// CreateSource persists a new top-level source. No fetch happens until a feed
// references it.
func (p *LiveProvider) CreateSource(_ context.Context, def SourceDef) (SourceDef, error) {
	return p.store.CreateSource(def)
}

// CreateFeed persists a new feed in the profile and returns its materialized
// summary. Fetch failures degrade to zero counts with a log line, matching
// materializeProfile.
func (p *LiveProvider) CreateFeed(ctx context.Context, profileID string, def FeedDef) (Source, error) {
	created, err := p.store.CreateFeed(profileID, def)
	if err != nil {
		return Source{}, err
	}
	summary := Source{ID: created.ID, Name: created.Name}
	srcByID, err := p.sourcesByID()
	if err != nil {
		return Source{}, err
	}
	items, err := p.feedItems(ctx, srcByID, created)
	if err != nil {
		p.logger.Debug().Err(err).Str("feed", created.ID).Msg("feed fetch failed; counting as empty")
		return summary, nil
	}
	summary.Count = len(items)
	for _, li := range items {
		if p.finalize(li).Unread {
			summary.NewCount++
		}
	}
	return summary, nil
}

// UpdateFeed replaces the feed's definition; the feed keeps its ID. The
// source cache is unaffected — filters apply at read time.
func (p *LiveProvider) UpdateFeed(_ context.Context, profileID, feedID string, def FeedDef) error {
	return p.store.UpdateFeed(profileID, feedID, def)
}

func (p *LiveProvider) Items(ctx context.Context, profileID, feedID string) ([]Item, error) {
	def, err := p.profileDef(profileID)
	if err != nil {
		return nil, err
	}

	feeds := def.Feeds
	if feedID != "" {
		feedDef, ok := findFeed(def.Feeds, feedID)
		if !ok {
			return nil, fmt.Errorf("feed: unknown feed %q in profile %q", feedID, profileID)
		}
		feeds = []FeedDef{feedDef}
	}

	merged, err := p.gather(ctx, def, feeds)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(merged))
	for _, li := range merged {
		items = append(items, p.finalize(li))
	}
	return items, nil
}

func (p *LiveProvider) ActionsFor(kind string) []Action {
	return ActionsFor(kind)
}

// MarkRead records the item as read as of its currently-known update time.
// The same item can sit in several source caches with different timestamps
// (search result vs notification thread); the newest wins so the item does
// not bounce back to unread.
func (p *LiveProvider) MarkRead(_ context.Context, _, itemID string) error {
	var updatedAt time.Time
	p.mu.Lock()
	for _, cached := range p.cache {
		for _, li := range cached.items {
			if li.item.ID == itemID && li.updatedAt.After(updatedAt) {
				updatedAt = li.updatedAt
			}
		}
	}
	p.mu.Unlock()
	if updatedAt.IsZero() {
		updatedAt = p.now()
	}
	return p.store.MarkRead(itemID, updatedAt)
}

// Config reports the profiles config file's location and current state.
func (p *LiveProvider) Config(context.Context) (ConfigInfo, error) {
	return p.store.ConfigInfo(), nil
}

// Invalidate drops the fetch cache so the next call refetches. Config
// reloads and auth changes call it: a notifications refetch is nearly free
// (conditional), and a search refetch is one request per source.
func (p *LiveProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*cachedSource)
}

// Refresh refetches the distinct sources referenced by the profile's feeds,
// bypassing the search TTL. Notifications sources still honor X-Poll-Interval
// — the conditional request is cheap, but the header is a contract. It
// reports whether any referenced source's data changed and fails only when
// every source fails.
func (p *LiveProvider) Refresh(ctx context.Context, profileID string) (bool, error) {
	def, err := p.profileDef(profileID)
	if err != nil {
		return false, err
	}
	srcByID, err := p.sourcesByID()
	if err != nil {
		return false, err
	}

	var (
		changed   bool
		lastErr   error
		refreshed int
	)
	for _, src := range distinctSources(def.Feeds, srcByID) {
		sourceChanged, err := p.refreshSource(ctx, src)
		if err != nil {
			lastErr = err
			p.logger.Debug().Err(err).Str("profile", def.Name).Str("source", src.ID).Msg("refresh fetch failed")
			continue
		}
		refreshed++
		if sourceChanged {
			changed = true
		}
	}
	if refreshed == 0 && lastErr != nil {
		return false, lastErr
	}
	return changed, nil
}

// refreshSource force-fetches one source, bypassing the search TTL, and
// reports whether its cached items changed. Notifications sources within
// their X-Poll-Interval window are skipped unchanged.
func (p *LiveProvider) refreshSource(ctx context.Context, src SourceDef) (bool, error) {
	key := sourceKey(src)
	p.mu.Lock()
	prev, hadPrev := p.cache[key]
	p.mu.Unlock()

	if hadPrev && src.Kind == "notifications" {
		if p.now().Sub(prev.fetchedAt) < notificationsTTL(prev) {
			return false, nil
		}
	}

	items, err := p.fetchSource(ctx, src)
	if err != nil {
		return false, err
	}
	return !hadPrev || !sameItems(prev.items, items), nil
}

// notificationsTTL is the effective minimum interval between notification
// fetches for a cache entry.
func notificationsTTL(cached *cachedSource) time.Duration {
	if cached.pollInterval > notificationsMinPoll {
		return cached.pollInterval
	}
	return notificationsMinPoll
}

// sameItems reports whether two fetches carry the same source state: same
// items, same order, same update times and native unread flags.
func sameItems(a, b []liveItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].item.ID != b[i].item.ID ||
			!a[i].updatedAt.Equal(b[i].updatedAt) ||
			a[i].item.Unread != b[i].item.Unread {
			return false
		}
	}
	return true
}

// ── Materialization ──────────────────────────────────────────────────────────

// materializeProfile builds the wire profile, including per-feed and total
// counts. Fetch failures degrade to zero counts with a log line: the rail
// should render even when GitHub is unreachable.
func (p *LiveProvider) materializeProfile(ctx context.Context, srcByID map[string]SourceDef, def ProfileDef) Profile {
	profile := Profile{
		ID:            def.ID,
		Letter:        ProfileLetter(def.Name),
		Name:          def.Name,
		SourceSummary: fmt.Sprintf("GitHub · %d sources", len(distinctSources(def.Feeds, srcByID))),
	}

	newest := make(map[string]liveItem)
	for _, feedDef := range def.Feeds {
		source := Source{ID: feedDef.ID, Name: feedDef.Name}
		items, err := p.feedItems(ctx, srcByID, feedDef)
		if err != nil {
			p.logger.Debug().Err(err).Str("profile", def.Name).Str("feed", feedDef.ID).Msg("feed fetch failed; counting as empty")
		} else {
			source.Count = len(items)
			for _, li := range items {
				if p.finalize(li).Unread {
					source.NewCount++
				}
				if prev, ok := newest[li.item.ID]; ok {
					newest[li.item.ID] = mergeNewest(prev, li)
				} else {
					newest[li.item.ID] = li
				}
			}
		}
		profile.Feeds = append(profile.Feeds, source)
	}
	// Profile totals dedupe on the newest copy, matching gather and the
	// MarkRead newest-wins contract, so the rail and list counts agree.
	for _, li := range newest {
		profile.TotalCount++
		if p.finalize(li).Unread {
			profile.UnreadCount++
		}
	}
	return profile
}

// gather materializes the given feeds, deduplicates by item ID (a PR can be
// in two feeds), and orders newest-updated first. It fails only when every
// feed fails; partial failures degrade with a log.
func (p *LiveProvider) gather(ctx context.Context, def ProfileDef, feeds []FeedDef) ([]liveItem, error) {
	srcByID, err := p.sourcesByID()
	if err != nil {
		return nil, err
	}

	var (
		merged  []liveItem
		index   = make(map[string]int)
		lastErr error
		fetched int
	)
	for _, feedDef := range feeds {
		items, err := p.feedItems(ctx, srcByID, feedDef)
		if err != nil {
			lastErr = err
			p.logger.Debug().Err(err).Str("profile", def.Name).Str("feed", feedDef.ID).Msg("feed fetch failed")
			continue
		}
		fetched++
		for _, li := range items {
			if at, ok := index[li.item.ID]; ok {
				merged[at] = mergeNewest(merged[at], li)
				continue
			}
			index[li.item.ID] = len(merged)
			merged = append(merged, li)
		}
	}
	if fetched == 0 && lastErr != nil {
		return nil, lastErr
	}

	sortNewestFirst(merged)
	return merged, nil
}

// mergeNewest resolves an item present in several sources (search result vs
// notification thread) to its newest copy, so age and read state derive from
// the latest activity — the same newest-wins contract MarkRead records.
// Notifications carry no author or labels, and search results no reason;
// backfill each empty field from the other copy so a merged item has author,
// labels, and reason regardless of which side is newer.
func mergeNewest(a, b liveItem) liveItem {
	newer, older := a, b
	if older.updatedAt.After(newer.updatedAt) {
		newer, older = older, newer
	}
	if newer.item.Author == "" {
		newer.item.Author = older.item.Author
	}
	if newer.item.Reason == "" {
		newer.item.Reason = older.item.Reason
	}
	if len(newer.item.Labels) == 0 {
		newer.item.Labels = older.item.Labels
	}
	return newer
}

// finalize applies app-local read state at materialization time, so cached
// fetches still reflect the latest MarkRead calls.
func (p *LiveProvider) finalize(li liveItem) Item {
	item := li.item
	if item.Unread {
		if readAt, ok := p.store.ReadAt(item.ID); ok && !li.updatedAt.After(readAt) {
			item.Unread = false
		}
	}
	return item
}

// ── Fetching ─────────────────────────────────────────────────────────────────

// feedItems materializes one feed: union the cached/fetched items of its
// sources, dedupe and merge by item ID, apply the feed's filters, and order
// newest first. It fails only when every source fails.
func (p *LiveProvider) feedItems(ctx context.Context, srcByID map[string]SourceDef, def FeedDef) ([]liveItem, error) {
	var (
		merged  []liveItem
		index   = make(map[string]int)
		lastErr error
		fetched int
	)
	for _, sourceID := range def.Sources {
		src, ok := srcByID[sourceID]
		if !ok {
			// Possible only in the window between a config reload and this
			// read racing on split store snapshots; the next poll heals it.
			lastErr = fmt.Errorf("feed: unknown source %q", sourceID)
			p.logger.Debug().Str("feed", def.ID).Str("source", sourceID).Msg("source not found; skipping")
			continue
		}
		items, err := p.sourceItems(ctx, src)
		if err != nil {
			lastErr = err
			p.logger.Debug().Err(err).Str("feed", def.ID).Str("source", src.ID).Msg("source fetch failed")
			continue
		}
		fetched++
		for _, li := range items {
			if at, ok := index[li.item.ID]; ok {
				merged[at] = mergeNewest(merged[at], li)
				continue
			}
			index[li.item.ID] = len(merged)
			merged = append(merged, li)
		}
	}
	if fetched == 0 && lastErr != nil {
		return nil, lastErr
	}

	sortNewestFirst(merged)
	return applyFilters(def, merged), nil
}

// sourceItems returns the source's items, from cache within the TTL — 30s
// for search, the X-Poll-Interval contract for notifications — and fetching
// otherwise.
func (p *LiveProvider) sourceItems(ctx context.Context, src SourceDef) ([]liveItem, error) {
	key := sourceKey(src)

	p.mu.Lock()
	cached, ok := p.cache[key]
	p.mu.Unlock()
	if ok {
		ttl := liveCacheTTL
		if src.Kind == "notifications" {
			ttl = notificationsTTL(cached)
		}
		if p.now().Sub(cached.fetchedAt) < ttl {
			return cached.items, nil
		}
	}

	items, err := p.fetchSource(ctx, src)
	if err != nil {
		// Serve stale data over an error when we have it: triage beats
		// freshness during a blip. Auth failures pass through even so —
		// stale data would mask the reconnect prompt.
		if ok && !errors.Is(err, github.ErrUnauthorized) && !errors.Is(err, ErrNotAuthenticated) {
			p.logger.Debug().Err(err).Str("source", src.ID).Msg("source fetch failed; serving stale cache")
			return cached.items, nil
		}
		return nil, err
	}
	return items, nil
}

// fetchSource performs the source's API request and updates the cache,
// coalescing concurrent fetches of the same source into one request — a poll
// tick racing a user navigation costs one API call, and both callers see the
// same result. A TTL-bypassing refresh that joins an in-flight fetch adopts
// its result: the data is at most one request old, fresh enough for a manual
// refresh.
func (p *LiveProvider) fetchSource(ctx context.Context, src SourceDef) ([]liveItem, error) {
	result, err, _ := p.flight.Do(sourceKey(src), func() (any, error) {
		return p.fetchSourceDirect(ctx, src)
	})
	if err != nil {
		return nil, err
	}
	return result.([]liveItem), nil
}

// fetchSourceDirect is the uncoalesced fetch behind fetchSource.
// Notifications requests are conditional: a 304 keeps the cached items and
// refreshes their fetch time at no rate-limit cost.
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
			// Published entries are immutable (readers use them outside the
			// lock): publish a refreshed copy instead of mutating prev. This
			// also re-adopts the entry if Invalidate raced the request.
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
		result, err := client.SearchIssues(ctx, src.Query, src.effectiveLimit())
		if err != nil {
			return nil, err
		}
		items := p.searchItems(result.Items)
		p.setCache(key, &cachedSource{items: items, fetchedAt: p.now()})
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

func (p *LiveProvider) searchItems(items []github.SearchItem) []liveItem {
	out := make([]liveItem, 0, len(items))
	for _, si := range items {
		kind := "Issue"
		if si.IsPullRequest() {
			kind = "PR"
		}
		repo := si.Repo()
		item := Item{
			ID:     itemID(repo, si.Number),
			Kind:   kind,
			Repo:   repo,
			Num:    si.Number,
			Title:  si.Title,
			Author: si.User.Login,
			Age:    shortAge(p.now().Sub(si.UpdatedAt)),
			Unread: true, // inbox model: unread until read locally
			Labels: labelNames(si.Labels),
			Branch: suggestedBranch(kind, si.Number, si.Title),
			Body:   si.Body,
			Prompt: suggestedPrompt(kind, repo, si.Number, si.Title),
			URL:    si.HTMLURL,
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
			// Releases, discussions, CI activity: out of scope for a
			// PR/issue triage feed until the UI has a kind for them.
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
			Unread: n.Unread, // native inbox state seeds unread
			Reason: n.Reason,
			// No Labels: the notifications API does not deliver them. The
			// reason renders as its own chip from Reason, and mergeNewest
			// backfills real labels from a search copy of the same item.
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

func (p *LiveProvider) profileDef(profileID string) (ProfileDef, error) {
	defs, err := p.store.Profiles()
	if err != nil {
		return ProfileDef{}, err
	}
	for _, def := range defs {
		if def.ID == profileID {
			return def, nil
		}
	}
	return ProfileDef{}, fmt.Errorf("feed: unknown profile %q", profileID)
}

func (p *LiveProvider) sourcesByID() (map[string]SourceDef, error) {
	sources, err := p.store.Sources()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]SourceDef, len(sources))
	for _, src := range sources {
		byID[src.ID] = src
	}
	return byID, nil
}

// distinctSources resolves the sources referenced by the feeds, deduplicated
// by canonical key, in first-reference order. Unresolvable references are
// skipped; validation prevents them in a consistent config snapshot.
func distinctSources(feeds []FeedDef, srcByID map[string]SourceDef) []SourceDef {
	var out []SourceDef
	seen := make(map[string]bool)
	for _, feedDef := range feeds {
		for _, sourceID := range feedDef.Sources {
			src, ok := srcByID[sourceID]
			if !ok {
				continue
			}
			key := sourceKey(src)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, src)
		}
	}
	return out
}

func sortNewestFirst(items []liveItem) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].updatedAt.After(items[j].updatedAt)
	})
}

func findFeed(feeds []FeedDef, id string) (FeedDef, bool) {
	for _, def := range feeds {
		if def.ID == id {
			return def, true
		}
	}
	return FeedDef{}, false
}

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

// suggestedBranch proposes a session branch name for acting on the item,
// mirroring the sources template convention (kind prefix, number, slug).
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
