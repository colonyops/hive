package feed

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/github"
)

// ErrNotAuthenticated is returned when no GitHub token is available. The UI
// never hits it in normal flow (the feed is gated behind auth), but the
// taxonomy keeps the failure explicit.
var ErrNotAuthenticated = errors.New("feed: not authenticated")

const (
	// liveCacheTTL keeps feed switches snappy between poller refreshes
	// without hammering the API.
	liveCacheTTL = 30 * time.Second
	// searchLimit bounds items per feed; the desktop feed is a triage
	// surface, not an archive browser.
	searchLimit = 30
)

// LiveProvider serves feed data from the GitHub API, with app-local unread
// state. Fetches are cached per (profile, feed) for liveCacheTTL.
type LiveProvider struct {
	client *github.Client
	tokens github.TokenStore
	store  *Store
	logger zerolog.Logger
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]*cachedFeed
}

type cachedFeed struct {
	items     []liveItem
	fetchedAt time.Time
}

// liveItem pairs the wire item with the timestamp unread state derives from.
type liveItem struct {
	item      Item
	updatedAt time.Time
}

func NewLiveProvider(client *github.Client, tokens github.TokenStore, store *Store, logger zerolog.Logger) *LiveProvider {
	return &LiveProvider{
		client: client,
		tokens: tokens,
		store:  store,
		logger: logger,
		now:    time.Now,
		cache:  make(map[string]*cachedFeed),
	}
}

func (p *LiveProvider) Profiles(ctx context.Context) ([]Profile, error) {
	defs, err := p.store.Profiles()
	if err != nil {
		return nil, err
	}

	profiles := make([]Profile, 0, len(defs))
	for _, def := range defs {
		profiles = append(profiles, p.materializeProfile(ctx, def))
	}
	return profiles, nil
}

// CreateProfile persists a new profile with default feeds and returns it
// materialized (fetching its feeds).
func (p *LiveProvider) CreateProfile(ctx context.Context, name string) (Profile, error) {
	def, err := p.store.CreateProfile(name)
	if err != nil {
		return Profile{}, err
	}
	return p.materializeProfile(ctx, def), nil
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
// The same item can sit in several feed caches with different timestamps
// (search result vs notification thread); the newest wins so the item does
// not bounce back to unread.
func (p *LiveProvider) MarkRead(_ context.Context, profileID, itemID string) error {
	var updatedAt time.Time
	p.mu.Lock()
	for key, cached := range p.cache {
		if !strings.HasPrefix(key, profileID+"\x00") {
			continue
		}
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

// Invalidate drops the fetch cache so the next call refetches.
func (p *LiveProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]*cachedFeed)
}

// Refresh refetches every feed of the profile, bypassing the TTL, and
// reports whether anything changed versus the cached state. The poller uses
// it to decide when to push feed:updated. It returns an error only when
// every feed fails.
func (p *LiveProvider) Refresh(ctx context.Context, profileID string) (bool, error) {
	def, err := p.profileDef(profileID)
	if err != nil {
		return false, err
	}

	var (
		changed bool
		lastErr error
		fetched int
	)
	for _, feedDef := range def.Feeds {
		items, err := p.fetchFeed(ctx, feedDef)
		if err != nil {
			lastErr = err
			p.logger.Debug().Err(err).Str("profile", def.Name).Str("feed", feedDef.ID).Msg("poll fetch failed")
			continue
		}
		fetched++

		key := def.ID + "\x00" + feedDef.ID
		p.mu.Lock()
		if prev, ok := p.cache[key]; !ok || !sameItems(prev.items, items) {
			changed = true
		}
		p.cache[key] = &cachedFeed{items: items, fetchedAt: p.now()}
		p.mu.Unlock()
	}
	if fetched == 0 && lastErr != nil {
		return false, lastErr
	}
	return changed, nil
}

// sameItems reports whether two fetches carry the same feed state: same
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
// counts. Feed fetch failures degrade to zero counts with a log line: the
// rail should render even when GitHub is unreachable.
func (p *LiveProvider) materializeProfile(ctx context.Context, def ProfileDef) Profile {
	profile := Profile{
		ID:            def.ID,
		Letter:        ProfileLetter(def.Name),
		Name:          def.Name,
		SourceSummary: fmt.Sprintf("GitHub · %d sources", len(def.Feeds)),
	}

	newest := make(map[string]liveItem)
	for _, feedDef := range def.Feeds {
		source := Source{ID: feedDef.ID, Name: feedDef.Name}
		items, err := p.feedItems(ctx, def.ID, feedDef)
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

// gather fetches the given feeds, deduplicates by item ID (a PR can be in a
// search feed and the notifications inbox), and orders newest-updated first.
// It fails only when every feed fails; partial failures degrade with a log.
func (p *LiveProvider) gather(ctx context.Context, def ProfileDef, feeds []FeedDef) ([]liveItem, error) {
	var (
		merged  []liveItem
		index   = make(map[string]int)
		lastErr error
		fetched int
	)
	for _, feedDef := range feeds {
		items, err := p.feedItems(ctx, def.ID, feedDef)
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

	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].updatedAt.After(merged[j].updatedAt)
	})
	return merged, nil
}

// mergeNewest resolves an item present in several feeds (search result vs
// notification thread) to its newest copy, so age and read state derive from
// the latest activity — the same newest-wins contract MarkRead records.
// Notifications carry no author; backfill it from the older copy.
func mergeNewest(a, b liveItem) liveItem {
	newer, older := a, b
	if older.updatedAt.After(newer.updatedAt) {
		newer, older = older, newer
	}
	if newer.item.Author == "" {
		newer.item.Author = older.item.Author
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

func (p *LiveProvider) feedItems(ctx context.Context, profileID string, def FeedDef) ([]liveItem, error) {
	key := profileID + "\x00" + def.ID

	p.mu.Lock()
	cached, ok := p.cache[key]
	p.mu.Unlock()
	if ok && p.now().Sub(cached.fetchedAt) < liveCacheTTL {
		return cached.items, nil
	}

	items, err := p.fetchFeed(ctx, def)
	if err != nil {
		// Serve stale data over an error when we have it: triage beats
		// freshness during a blip. Auth failures pass through even so —
		// stale data would mask the reconnect prompt.
		if ok && !errors.Is(err, github.ErrUnauthorized) && !errors.Is(err, ErrNotAuthenticated) {
			p.logger.Debug().Err(err).Str("feed", def.ID).Msg("feed fetch failed; serving stale cache")
			return cached.items, nil
		}
		return nil, err
	}

	p.mu.Lock()
	p.cache[key] = &cachedFeed{items: items, fetchedAt: p.now()}
	p.mu.Unlock()
	return items, nil
}

func (p *LiveProvider) fetchFeed(ctx context.Context, def FeedDef) ([]liveItem, error) {
	token, err := p.tokens.Token()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrNotAuthenticated
	}
	client := p.client.WithTokenCopy(token)

	switch def.Kind {
	case "notifications":
		notifications, err := client.Notifications(ctx, searchLimit)
		if err != nil {
			return nil, err
		}
		return p.notificationItems(notifications), nil
	case "search":
		result, err := client.SearchIssues(ctx, def.Query, searchLimit)
		if err != nil {
			return nil, err
		}
		return p.searchItems(result.Items), nil
	default:
		return nil, fmt.Errorf("feed: unknown feed kind %q", def.Kind)
	}
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
			Labels: []string{strings.ReplaceAll(n.Reason, "_", " ")},
			Branch: suggestedBranch(kind, num, n.Subject.Title),
			Body:   fmt.Sprintf("GitHub notification (%s) for %s in %s.", strings.ReplaceAll(n.Reason, "_", " "), strings.ToLower(kind), repo),
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
