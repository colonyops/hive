package main

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixedStore(t *testing.T, overlays ...Overlay) *Store {
	t.Helper()
	s := NewStore(overlays)
	s.now = func() time.Time { return time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC) }
	return s
}

// graphQLBody is a minimal but shape-accurate batched search response: two
// aliased searches, mixed issue and pull-request nodes.
const graphQLBody = `{"data":{
  "s0":{"nodes":[
    {"__typename":"PullRequest","number":58,"title":"Strip inline markdown","body":"b","state":"OPEN",
     "url":"https://github.com/hay-kot/hive-desktop/pull/58","isDraft":false,
     "createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-24T00:00:00Z",
     "author":{"login":"hay-kot"},"repository":{"nameWithOwner":"hay-kot/hive-desktop"},
     "labels":{"nodes":[{"name":"chore"}]}},
    {"__typename":"Issue","number":12,"title":"Other","body":"","state":"OPEN",
     "url":"https://github.com/hay-kot/hive-desktop/issues/12",
     "createdAt":"2026-07-20T00:00:00Z","updatedAt":"2026-07-24T00:00:00Z",
     "author":{"login":"hay-kot"},"repository":{"nameWithOwner":"hay-kot/hive-desktop"},
     "labels":{"nodes":[]}}]},
  "s1":{"nodes":[]}}}`

func graphQLNodes(t *testing.T, body []byte, alias string) []map[string]any {
	t.Helper()
	var payload struct {
		Data map[string]struct {
			Nodes []map[string]any `json:"nodes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload.Data[alias].Nodes
}

// nodeNumber reads a search node's number, which JSON decodes as a float64.
func nodeNumber(t *testing.T, node map[string]any) int {
	t.Helper()
	raw, ok := node["number"].(float64)
	require.True(t, ok, "node has no numeric number field")
	return int(raw)
}

func TestRewriteGraphQLNoOverlayLeavesBodyUntouched(t *testing.T) {
	store := fixedStore(t)
	out := store.RewriteGraphQL([]byte(graphQLBody))
	assert.True(t, bytes.Equal([]byte(graphQLBody), out),
		"an unmatched body must be returned byte-identical, not re-marshalled")
}

func TestRewriteGraphQLObservesItemsWithoutOverlay(t *testing.T) {
	store := fixedStore(t)
	store.RewriteGraphQL([]byte(graphQLBody))

	items := store.Items()
	require.Len(t, items, 2)
	byKey := map[string]Item{}
	for _, item := range items {
		byKey[item.Repo+"#"+strconv.Itoa(item.Num)] = item
	}
	assert.Equal(t, "PR", byKey["hay-kot/hive-desktop#58"].Kind)
	assert.Equal(t, "open", byKey["hay-kot/hive-desktop#58"].State)
	assert.Equal(t, "Strip inline markdown", byKey["hay-kot/hive-desktop#58"].Title)
	assert.Equal(t, "Issue", byKey["hay-kot/hive-desktop#12"].Kind)
	assert.False(t, byKey["hay-kot/hive-desktop#58"].Overlaid)
}

func TestRewriteGraphQLAppliesMutations(t *testing.T) {
	store := fixedStore(t)
	labels := []string{"needs-review", "urgent"}
	store.Apply("hay-kot/hive-desktop#58", Mutations{
		State:  new("closed"),
		Labels: &labels,
		Title:  new("Rewritten"),
		Draft:  new(true),
	})

	nodes := graphQLNodes(t, store.RewriteGraphQL([]byte(graphQLBody)), "s0")
	require.Len(t, nodes, 2, "a non-absent overlay must not drop the node")

	assert.Equal(t, "CLOSED", nodes[0]["state"], "GraphQL spells state in upper case")
	assert.Equal(t, "Rewritten", nodes[0]["title"])
	assert.Equal(t, true, nodes[0]["isDraft"])
	assert.Equal(t, map[string]any{"nodes": []any{
		map[string]any{"name": "needs-review"},
		map[string]any{"name": "urgent"},
	}}, nodes[0]["labels"])

	// The unmatched node must be identical to its input.
	assert.Equal(t, "OPEN", nodes[1]["state"])
	assert.Equal(t, "Other", nodes[1]["title"])
}

func TestRewriteGraphQLStampsUpdatedAtWhenUnset(t *testing.T) {
	store := fixedStore(t)
	store.Apply("hay-kot/hive-desktop#58", Mutations{Reason: new("comment")})

	nodes := graphQLNodes(t, store.RewriteGraphQL([]byte(graphQLBody)), "s0")
	// The desktop classifier ignores any change that does not advance
	// updatedAt, so an un-stamped mutation would be silently invisible.
	assert.Equal(t, "2026-07-25T12:00:00Z", nodes[0]["updatedAt"])
}

func TestRewriteGraphQLHonorsExplicitUpdatedAt(t *testing.T) {
	store := fixedStore(t)
	explicit := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	store.Apply("hay-kot/hive-desktop#58", Mutations{UpdatedAt: &explicit})

	nodes := graphQLNodes(t, store.RewriteGraphQL([]byte(graphQLBody)), "s0")
	assert.Equal(t, "2026-01-02T03:04:05Z", nodes[0]["updatedAt"])
}

func TestRewriteGraphQLAbsentDropsNode(t *testing.T) {
	store := fixedStore(t)
	store.Apply("hay-kot/hive-desktop#58", Mutations{Absent: new(true)})

	nodes := graphQLNodes(t, store.RewriteGraphQL([]byte(graphQLBody)), "s0")
	require.Len(t, nodes, 1, "an absent item must leave the search result")
	assert.Equal(t, 12, nodeNumber(t, nodes[0]), "the surviving node must be the unmatched one")
}

func TestRewriteGraphQLStateLookup(t *testing.T) {
	store := fixedStore(t)
	store.Apply("hay-kot/hive-desktop#58", Mutations{State: new("merged"), Absent: new(true)})

	body := `{"data":{"r0":{"issueOrPullRequest":{"__typename":"PullRequest","number":58,"state":"OPEN",
	  "updatedAt":"2026-07-24T00:00:00Z","repository":{"nameWithOwner":"hay-kot/hive-desktop"}}}}}`

	var payload struct {
		Data map[string]struct {
			IssueOrPullRequest map[string]any `json:"issueOrPullRequest"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(store.RewriteGraphQL([]byte(body)), &payload))

	node, ok := payload.Data["r0"]
	require.True(t, ok, "the r0 alias must not be dropped despite absent:true")
	require.NotNil(t, node.IssueOrPullRequest)
	assert.Equal(t, "MERGED", node.IssueOrPullRequest["state"], "absent is not honoured on the state-lookup shape")
}

func TestRewriteGraphQLMalformedBodyIsReturnedUnchanged(t *testing.T) {
	store := fixedStore(t)
	store.Apply("hay-kot/hive-desktop#58", Mutations{State: new("closed")})

	for _, body := range []string{"not json", "", "[1,2,3]", `{"errors":[{"message":"boom"}]}`} {
		assert.True(t, bytes.Equal([]byte(body), store.RewriteGraphQL([]byte(body))),
			"the proxy must never turn a response it cannot parse into a broken one")
	}
}

const notificationsBody = `[
  {"id":"1","unread":false,"reason":"subscribed","updated_at":"2026-07-24T00:00:00Z",
   "subject":{"title":"Strip inline markdown","type":"PullRequest",
              "url":"https://api.github.com/repos/hay-kot/hive-desktop/pulls/58"},
   "repository":{"full_name":"hay-kot/hive-desktop"}},
  {"id":"2","unread":true,"reason":"mention","updated_at":"2026-07-24T00:00:00Z",
   "subject":{"title":"Release notes","type":"Release",
              "url":"https://api.github.com/repos/hay-kot/hive-desktop/releases/9"},
   "repository":{"full_name":"hay-kot/hive-desktop"}}]`

func notificationEntries(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var entries []map[string]any
	require.NoError(t, json.Unmarshal(body, &entries))
	return entries
}

func TestRewriteNotificationsSetsReasonAndMarksUnread(t *testing.T) {
	store := fixedStore(t)
	store.Apply("hay-kot/hive-desktop#58", Mutations{Reason: new("approval_requested")})

	entries := notificationEntries(t, store.RewriteNotifications([]byte(notificationsBody)))
	require.Len(t, entries, 2)
	assert.Equal(t, "approval_requested", entries[0]["reason"])
	assert.Equal(t, true, entries[0]["unread"],
		"a simulated event the user has not seen must be unread or the feed hides it")
	assert.Equal(t, "2026-07-25T12:00:00Z", entries[0]["updated_at"])

	assert.Equal(t, "mention", entries[1]["reason"], "unmatched entries stay untouched")
}

func TestRewriteNotificationsIgnoresNonItemSubjects(t *testing.T) {
	store := fixedStore(t)
	store.RewriteNotifications([]byte(notificationsBody))

	// The Release subject has no item number, so it must not be observed as
	// an item the dashboard offers actions for.
	for _, item := range store.Items() {
		assert.NotEqual(t, 9, item.Num)
	}
}

func TestMergeSimulationIsConsistentAcrossShapes(t *testing.T) {
	// The whole point of one overlay driving every response shape: a merge has
	// to leave the search result *and* have the state lookup report it merged.
	// Rewriting only one shape produces a state the real API cannot return.
	store := fixedStore(t)
	store.Apply("hay-kot/hive-desktop#58", quickActions["merge"].apply())

	nodes := graphQLNodes(t, store.RewriteGraphQL([]byte(graphQLBody)), "s0")
	require.Len(t, nodes, 1)
	assert.Equal(t, 12, nodeNumber(t, nodes[0]), "the merged PR must be gone from search")

	stateLookupBody := `{"data":{"r0":{"issueOrPullRequest":{"__typename":"PullRequest","number":58,"state":"OPEN",
	  "updatedAt":"2026-07-24T00:00:00Z","repository":{"nameWithOwner":"hay-kot/hive-desktop"}}}}}`
	var payload struct {
		Data map[string]struct {
			IssueOrPullRequest map[string]any `json:"issueOrPullRequest"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(store.RewriteGraphQL([]byte(stateLookupBody)), &payload))
	assert.Equal(t, "MERGED", payload.Data["r0"].IssueOrPullRequest["state"],
		"the state lookup ConfirmTerminal calls must confirm the merge")
}

func TestStoreApplyMergesSuccessiveMutations(t *testing.T) {
	store := fixedStore(t)
	store.Apply("a/b#1", Mutations{Reason: new("review_requested")})
	merged := store.Apply("a/b#1", Mutations{State: new("closed")})

	require.NotNil(t, merged.Reason)
	assert.Equal(t, "review_requested", *merged.Reason, "an earlier field must survive a later mutation")
	require.NotNil(t, merged.State)
	assert.Equal(t, "closed", *merged.State)
}

func TestStoreClearRestoresUpstream(t *testing.T) {
	store := fixedStore(t)
	store.Apply("hay-kot/hive-desktop#58", Mutations{Absent: new(true)})
	store.Clear("hay-kot/hive-desktop#58")

	assert.True(t, bytes.Equal([]byte(graphQLBody), store.RewriteGraphQL([]byte(graphQLBody))))
}

func TestStoreSeedsObservedItemsFromConfigOverlays(t *testing.T) {
	store := fixedStore(t, Overlay{
		Match: Matcher{Repo: "acme/widgets", Num: 3},
		Set:   Mutations{State: new("closed")},
	})
	items := store.Items()
	require.Len(t, items, 1, "a configured overlay must be visible before its item is ever seen")
	assert.Equal(t, "acme/widgets#3", items[0].Repo+"#"+strconv.Itoa(items[0].Num))
	assert.True(t, items[0].Overlaid)
}

func TestNumberFromAPIURL(t *testing.T) {
	cases := map[string]int{
		"https://api.github.com/repos/o/r/pulls/58": 58,
		"https://api.github.com/repos/o/r/issues/1": 1,
		"https://api.github.com/repos/o/r/pulls/":   0,
		"not-a-url": 0,
		"":          0,
		// Non-item subjects number their own collections; reading those as
		// item numbers would collide with a real issue of the same number.
		"https://api.github.com/repos/o/r/releases/9":    0,
		"https://api.github.com/repos/o/r/discussions/4": 0,
		"https://api.github.com/repos/o/r/commits/abc":   0,
	}
	for url, want := range cases {
		assert.Equal(t, want, numberFromAPIURL(url), url)
	}
}

func TestItemsSortOverlaidFirst(t *testing.T) {
	store := fixedStore(t)
	base := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	for i, key := range []string{"a/b#1", "a/b#2", "a/b#3"} {
		store.now = func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		parts := strings.SplitN(key, "#", 2)
		num, _ := strconv.Atoi(parts[1])
		store.observe(parts[0], num, "PR", "t", "open")
	}
	// The oldest-seen item is the overlaid one, so recency alone would bury it.
	store.Apply("a/b#1", Mutations{State: new("closed")})

	items := store.Items()
	require.Len(t, items, 3)
	assert.Equal(t, "a/b#1", items[0].Repo+"#"+strconv.Itoa(items[0].Num), "the item being simulated must be findable at the top")
	assert.True(t, items[0].Overlaid)
	assert.False(t, items[1].Overlaid)
}

func TestObservedItemsAreBounded(t *testing.T) {
	store := fixedStore(t)
	base := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	for i := range maxObservedItems + 50 {
		store.now = func() time.Time { return base.Add(time.Duration(i) * time.Second) }
		store.observe("a/b", i+1, "PR", "t", "open")
	}
	assert.Len(t, store.Items(), maxObservedItems,
		"a long-running devserver must not accumulate items indefinitely")

	// Eviction is least-recently-seen, so the earliest items are the ones gone.
	keys := map[string]bool{}
	for _, item := range store.Items() {
		keys[item.Repo+"#"+strconv.Itoa(item.Num)] = true
	}
	assert.False(t, keys["a/b#1"])
	assert.True(t, keys["a/b#"+strconv.Itoa(maxObservedItems+50)])
}

func TestOverlaidItemsSurviveEviction(t *testing.T) {
	store := fixedStore(t)
	base := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	// Observed first, so least-recently-seen, and overlaid — it must survive
	// anyway. Evicting it would hide an item whose overlay is still in force.
	store.now = func() time.Time { return base }
	store.observe("a/b", 1, "PR", "pinned", "open")
	store.Apply("a/b#1", Mutations{State: new("merged")})

	for i := range maxObservedItems + 50 {
		store.now = func() time.Time { return base.Add(time.Duration(i+1) * time.Second) }
		store.observe("c/d", i+1, "PR", "t", "open")
	}

	keys := map[string]bool{}
	for _, item := range store.Items() {
		keys[item.Repo+"#"+strconv.Itoa(item.Num)] = true
	}
	assert.True(t, keys["a/b#1"], "an overlaid item must never be evicted")
}
