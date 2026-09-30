package main

import (
	"encoding/json"
	"maps"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Store holds the live overlay state and the set of items the proxy has
// observed flowing through it. Config overlays seed it at startup; the control
// API and scenario runner mutate it afterwards. Nothing persists — a restart
// returns to exactly what devserver.yaml declares, so a debugging session
// cannot leave permanent fake data behind.
type Store struct {
	mu       sync.RWMutex
	overlays map[string]Mutations
	observed map[string]*Item
	now      func() time.Time
}

// Item is one item the proxy has seen in an upstream response, used to
// populate the dashboard without requiring the author to name items up front.
type Item struct {
	Repo     string    `json:"repo"`
	Num      int       `json:"num"`
	Kind     string    `json:"kind"`
	Title    string    `json:"title"`
	State    string    `json:"state"`
	LastSeen time.Time `json:"lastSeen"`
	// Overlaid reports whether an overlay currently applies to this item.
	Overlaid bool `json:"overlaid"`
}

func NewStore(overlays []Overlay) *Store {
	s := &Store{
		overlays: make(map[string]Mutations, len(overlays)),
		observed: make(map[string]*Item),
		now:      time.Now,
	}
	for _, overlay := range overlays {
		key := overlay.Match.Key()
		s.overlays[key] = s.overlays[key].Merge(overlay.Set)
		// Seed the item list from config so an overlay for an item the proxy
		// has not seen yet still shows on the dashboard.
		s.observed[key] = &Item{Repo: overlay.Match.Repo, Num: overlay.Match.Num, Kind: "Unknown"}
	}
	return s
}

// Apply merges mutations onto an item's overlay and returns the result. An
// UpdatedAt is stamped when the caller did not supply one: the desktop's
// classifier ignores any change that does not advance updatedAt
// (internal/app/sources/github/classify.go:82), so an un-stamped mutation
// would be invisible.
func (s *Store) Apply(key string, next Mutations) Mutations {
	s.mu.Lock()
	defer s.mu.Unlock()
	if next.UpdatedAt == nil && !next.Empty() {
		stamp := s.now().UTC()
		next.UpdatedAt = &stamp
	}
	merged := s.overlays[key].Merge(next)
	s.overlays[key] = merged
	return merged
}

// Clear drops an item's overlay, returning it to whatever upstream says.
func (s *Store) Clear(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.overlays, key)
}

// ClearAll drops every overlay.
func (s *Store) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overlays = make(map[string]Mutations)
}

// Get returns the overlay for a key, if any.
func (s *Store) Get(key string) (Mutations, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.overlays[key]
	return m, ok
}

// Overlays returns a snapshot of every active overlay.
func (s *Store) Overlays() map[string]Mutations {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Mutations, len(s.overlays))
	maps.Copy(out, s.overlays)
	return out
}

// maxObservedItems bounds the observed-item list. Every distinct item that
// ever flows through is remembered, so a long-running devserver watching a
// churning feed would accumulate indefinitely and hand the dashboard an
// ever-growing list. Well above a realistic feed's working set, so eviction
// stays a backstop rather than something you notice.
const maxObservedItems = 500

// observe records that an item passed through the proxy. The stored values are
// pre-overlay: the dashboard shows what upstream actually says alongside the
// overlay that is changing it.
func (s *Store) observe(repo string, num int, kind, title, state string) {
	if repo == "" || num <= 0 {
		return
	}
	key := repo + "#" + strconv.Itoa(num)
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.observed[key]
	if !ok {
		s.evictLocked()
		item = &Item{Repo: repo, Num: num}
		s.observed[key] = item
	}
	item.LastSeen = s.now().UTC()
	if kind != "" {
		item.Kind = kind
	}
	if title != "" {
		item.Title = title
	}
	if state != "" {
		item.State = state
	}
}

// evictLocked drops the least recently seen items once the list is full,
// making room for one new entry. Overlaid items are never evicted: they are
// what the author is actively simulating, and dropping one would silently
// remove it from the dashboard while its overlay stayed in force.
//
// Callers must hold s.mu.
func (s *Store) evictLocked() {
	if len(s.observed) < maxObservedItems {
		return
	}
	type aged struct {
		key  string
		seen time.Time
	}
	candidates := make([]aged, 0, len(s.observed))
	for key, item := range s.observed {
		if _, overlaid := s.overlays[key]; overlaid {
			continue
		}
		candidates = append(candidates, aged{key: key, seen: item.LastSeen})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].seen.Before(candidates[j].seen) })

	// Trim to one below the cap so the incoming item fits. If overlays account
	// for the whole list there is nothing to evict, and it grows past the cap
	// rather than discarding state the author asked for.
	for _, candidate := range candidates {
		if len(s.observed) < maxObservedItems {
			return
		}
		delete(s.observed, candidate.key)
	}
}

// Items returns every observed item with Overlaid set, overlaid items first
// and newest-seen within each group.
//
// A real feed observes hundreds of items, so the ones being actively
// simulated have to float to the top; ordering by recency alone buries them
// under whatever the last poll happened to return.
func (s *Store) Items() []Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Item, 0, len(s.observed))
	for key, item := range s.observed {
		copied := *item
		_, copied.Overlaid = s.overlays[key]
		out = append(out, copied)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Overlaid != out[j].Overlaid {
			return out[i].Overlaid
		}
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].Num < out[j].Num
	})
	return out
}

// ── Response rewriting ───────────────────────────────────────────────────────
//
// The desktop reads the same logical item through three different response
// shapes: a batched GraphQL search, a batched GraphQL state lookup, and REST
// notifications. All three are rewritten from one Mutations value so a
// simulated merge stays consistent: the item leaves the search result *and*
// the state lookup reports it merged. An overlay that only rewrote one shape
// would produce a state the real API can never return, and the bug would look
// like an app bug.

// RewriteGraphQL rewrites a GraphQL response body in place. The desktop sends
// two aliased document shapes under this one endpoint — a batched search
// (`data.sN.nodes[]`) and a batched state lookup
// (`data.rN.issueOrPullRequest`) — and a single response can only ever be one
// of them, so both are tried per top-level value. Search nodes are matched by
// repository.nameWithOwner + number, dropped when their overlay marks them
// absent, and the rewritten body is returned. A body it cannot parse is
// returned unchanged — the proxy must never turn a valid upstream response
// into a broken one.
func (s *Store) RewriteGraphQL(body []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return body
	}

	changed := false
	for _, raw := range data {
		result, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if s.rewriteItemAlias(result) {
			changed = true
			continue
		}
		nodes, ok := result["nodes"].([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(nodes))
		for _, rawNode := range nodes {
			node, ok := rawNode.(map[string]any)
			if !ok {
				kept = append(kept, rawNode)
				continue
			}
			repo, num := graphQLIdentity(node)
			kind := "Issue"
			if typename, _ := node["__typename"].(string); typename == "PullRequest" {
				kind = "PR"
			}
			title, _ := node["title"].(string)
			state, _ := node["state"].(string)
			s.observe(repo, num, kind, title, strings.ToLower(state))

			overlay, ok := s.Get(repo + "#" + strconv.Itoa(num))
			if !ok {
				kept = append(kept, node)
				continue
			}
			changed = true
			if overlay.Absent != nil && *overlay.Absent {
				continue
			}
			applyGraphQLNode(node, overlay)
			kept = append(kept, node)
		}
		if len(kept) != len(nodes) {
			changed = true
		}
		result["nodes"] = kept
	}
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
}

// An item marked absent is still answered here: leaving a query is what
// "absent" means, and the state lookup is how the app finds out why.
func (s *Store) rewriteItemAlias(result map[string]any) bool {
	node, ok := result["issueOrPullRequest"].(map[string]any)
	if !ok {
		return false
	}
	repo, num := graphQLIdentity(node)
	kind := "Issue"
	if typename, _ := node["__typename"].(string); typename == "PullRequest" {
		kind = "PR"
	}
	state, _ := node["state"].(string)
	s.observe(repo, num, kind, "", strings.ToLower(state))

	overlay, ok := s.Get(repo + "#" + strconv.Itoa(num))
	if !ok {
		return false
	}
	applyGraphQLNode(node, overlay)
	return true
}

// graphQLIdentity pulls the repo and number out of a search node.
func graphQLIdentity(node map[string]any) (repo string, num int) {
	if repository, ok := node["repository"].(map[string]any); ok {
		repo, _ = repository["nameWithOwner"].(string)
	}
	if raw, ok := node["number"].(float64); ok {
		num = int(raw)
	}
	return repo, num
}

// applyGraphQLNode writes the overlay onto one search node, using GraphQL's
// spelling: uppercase state, MERGED as a first-class state, labels as a
// {nodes:[{name}]} connection.
func applyGraphQLNode(node map[string]any, overlay Mutations) {
	if overlay.State != nil {
		node["state"] = strings.ToUpper(*overlay.State)
	}
	if overlay.Title != nil {
		node["title"] = *overlay.Title
	}
	if overlay.Body != nil {
		node["body"] = *overlay.Body
	}
	if overlay.Draft != nil {
		node["isDraft"] = *overlay.Draft
	}
	if overlay.Labels != nil {
		labelNodes := make([]any, 0, len(*overlay.Labels))
		for _, name := range *overlay.Labels {
			labelNodes = append(labelNodes, map[string]any{"name": name})
		}
		node["labels"] = map[string]any{"nodes": labelNodes}
	}
	if overlay.UpdatedAt != nil {
		node["updatedAt"] = overlay.UpdatedAt.UTC().Format(time.RFC3339)
	}
}

// RewriteNotifications rewrites a REST /notifications response. Notifications
// are the only shape carrying a reason, which is what the classifier turns
// into an activity summary ("Approval requested", "Review requested"), so this
// is the path a review-state simulation travels.
func (s *Store) RewriteNotifications(body []byte) []byte {
	var entries []map[string]any
	if err := json.Unmarshal(body, &entries); err != nil {
		return body
	}

	changed := false
	for _, entry := range entries {
		repo := ""
		if repository, ok := entry["repository"].(map[string]any); ok {
			repo, _ = repository["full_name"].(string)
		}
		subject, _ := entry["subject"].(map[string]any)
		num := 0
		kind := ""
		title := ""
		if subject != nil {
			url, _ := subject["url"].(string)
			num = numberFromAPIURL(url)
			title, _ = subject["title"].(string)
			switch subjectType, _ := subject["type"].(string); subjectType {
			case "PullRequest":
				kind = "PR"
			case "Issue":
				kind = "Issue"
			}
		}
		s.observe(repo, num, kind, title, "")

		overlay, ok := s.Get(repo + "#" + strconv.Itoa(num))
		if !ok {
			continue
		}
		changed = true
		if overlay.Reason != nil {
			entry["reason"] = *overlay.Reason
			// A reason the user has not seen is unread by definition; leaving
			// it read would hide the simulated event from the feed's inbox.
			entry["unread"] = true
		}
		if overlay.Title != nil && subject != nil {
			subject["title"] = *overlay.Title
		}
		if overlay.UpdatedAt != nil {
			entry["updated_at"] = overlay.UpdatedAt.UTC().Format(time.RFC3339)
		}
	}
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(entries)
	if err != nil {
		return body
	}
	return rewritten
}

// numberFromAPIURL extracts the item number from a GitHub API subject URL such
// as https://api.github.com/repos/o/r/pulls/58.
//
// The collection segment must be issues or pulls. Notification subjects also
// point at releases, discussions, and commits, which carry numbers of their
// own in the same position — .../releases/9 would otherwise be read as item
// #9 and collide with a real issue #9's overlay. Anything else returns 0,
// which observe and the overlay lookup both treat as "no item".
func numberFromAPIURL(url string) int {
	slash := strings.LastIndex(url, "/")
	if slash < 0 {
		return 0
	}
	collection := url[:slash]
	if idx := strings.LastIndex(collection, "/"); idx >= 0 {
		collection = collection[idx+1:]
	}
	if collection != "issues" && collection != "pulls" {
		return 0
	}
	num, err := strconv.Atoi(url[slash+1:])
	if err != nil || num <= 0 {
		return 0
	}
	return num
}
