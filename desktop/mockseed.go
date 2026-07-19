package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// MockFlowID and MockFeedNodeID identify the fixture flow's terminal feed
// node that seedMockFeedItems writes into: desktop/e2e/fixtures/flows/
// frontend-triage.yaml has a "feed" node with this exact id, and
// desktop/e2e/scripts/serve.sh points HIVE_DESKTOP_FLOWS at that fixture
// directory for the mock "feed" server. If either the fixture or these
// constants drift, FeedItemCounts/FeedItems (keyed off the flow's own feed
// nodes) simply won't find these rows — see mockseed_test.go, which loads
// the fixture flow and asserts the ids agree.
const (
	MockFlowID     = "frontend-triage"
	MockFeedNodeID = "notifications-inbox"
)

// mockFeedID is the flow-qualified feed key feed_item rows are upserted
// under ("<flowId>/<nodeId>"), matching how useFeedState.ts's loadFeeds
// derives a feed's id from its flow.
func mockFeedID() string {
	return MockFlowID + "/" + MockFeedNodeID
}

// mockFeedItems is the deterministic fixture item set the e2e suite
// (feed.spec.ts, theme.spec.ts, flows-editor.spec.ts) snapshots against.
// It reproduces the content of the old internal/desktop/feed/mock.go's
// static Items() list verbatim (deleted along with the rest of the legacy
// feed mock) — only the delivery mechanism changed: a real feed_item row
// instead of a hardcoded RPC response. Order matters: seedMockFeedItems
// stamps a strictly decreasing updated_at per index, so
// ListFeedItemsByFeed's "ORDER BY updated_at DESC" reproduces this exact
// slice order.
var mockFeedItems = []feed.Item{
	{
		ID: "pr2841", Kind: "PR", Repo: "hive/core", Num: 2841,
		Title:  "batch_spawn: fix detached tmux env & PATH propagation",
		Author: "lena", Age: "2h", Unread: true, Labels: []string{"bug", "batch"},
		Branch: "fix/2841-batch-spawn-env",
		Body:   "Sessions spawned from a GUI context inherit an empty PATH and lose HIVE_* vars, so batch_spawn fails to find the agent binary. Needs a controlled env when there is no controlling terminal.",
		Prompt: "Investigate detached tmux env in batch_spawn; ensure PATH and HIVE_* vars propagate when spawned headless from the desktop app.",
		URL:    "https://github.com/hive/core/pull/2841",
	},
	{
		ID: "iss1190", Kind: "Issue", Repo: "hive/desktop", Num: 1190,
		Title:  "Feed source: mirror GitHub notifications inbox",
		Author: "hayden", Age: "5h", Unread: true, Reason: "mention", Labels: []string{"feature", "mvp"},
		Branch: "feat/1190-notifications-feed",
		Body:   "Add a notifications-based feed source that mirrors the user's GitHub inbox, with local read/dismiss state so triage does not touch GitHub until the user acts.",
		Prompt: "Implement a notifications-based feed source mirroring the GitHub inbox, with app-local read/dismiss triage state.",
		URL:    "https://github.com/hive/desktop/issues/1190",
	},
	{
		ID: "pr2838", Kind: "PR", Repo: "hive/desktop", Num: 2838,
		Title:  "OAuth device flow for in-app GitHub auth",
		Author: "koji", Age: "1d", Unread: false, Labels: []string{"auth"},
		Branch: "feat/2838-oauth-device-flow",
		Body:   "Adds the full device-flow auth so users can sign in without leaving the app. Open question on GitHub App vs OAuth App registration and where to store the token.",
		Prompt: "Review the OAuth device-flow implementation and validate keychain token storage across platforms.",
		URL:    "https://github.com/hive/desktop/pull/2838",
	},
	{
		ID: "iss1204", Kind: "Issue", Repo: "hive/desktop", Num: 1204,
		Title:  "Composable view contract for feed / task / doc surfaces",
		Author: "mira", Age: "1d", Unread: true, Labels: []string{"arch"},
		Branch: "feat/1204-composable-views",
		Body:   "Define a self-contained component contract for feed, task, and doc views so a designer-led layout system can be dropped in later without rewrites.",
		Prompt: "Draft a composable, self-contained view interface covering the feed, task list, and doc viewer surfaces.",
		URL:    "https://github.com/hive/desktop/issues/1204",
	},
	{
		ID: "pr2830", Kind: "PR", Repo: "hive/core", Num: 2830,
		Title:  "Keychain-backed token storage",
		Author: "sam", Age: "2d", Unread: false, Labels: []string{"security"},
		Branch: "feat/2830-keychain-tokens",
		Body:   "Store GitHub tokens in the OS keychain instead of a plaintext config file, with a fallback for headless CI environments.",
		Prompt: "Review cross-platform keychain token storage and the headless fallback path.",
		URL:    "https://github.com/hive/core/pull/2830",
	},
	{
		ID: "iss1177", Kind: "Issue", Repo: "hive/desktop", Num: 1177,
		Title:  "Cross-repo query: PRs assigned to me across the org",
		Author: "hayden", Age: "3d", Unread: false, Labels: []string{"feature"},
		Branch: "feat/1177-cross-repo-query",
		Body:   "Support GitHub search-style cross-repo queries as a feed source, e.g. \"PRs assigned to me across the org\", saveable as a workspace source.",
		Prompt: "Implement a cross-repo query feed source using GitHub search syntax, saveable into a workspace.",
		URL:    "https://github.com/hive/desktop/issues/1177",
	},
}

// seedMockFeedItems upserts mockFeedItems into feed_item under mockFeedID(),
// so HIVE_DESKTOP_MOCK=feed serves a deterministic sidebar without a live
// producer (mock mode skips buildPipelineProducer/buildOutputWorker
// entirely — see main.go). It writes directly via db.Queries() rather than
// CommitBatch: CommitBatch stamps every Output in a batch with the same
// time.Now() call, which would leave every seeded row tied and ordering
// undefined, whereas the e2e specs (feed.spec.ts) assert an exact item
// order.
//
// Called once from main() in "feed" mock mode, after pipelineDB is open.
// Idempotent: UpsertFeedItem is keyed on (feed_id, item_id), so a restart
// simply re-applies the same fixture rows in place.
func seedMockFeedItems(db *pipelinedb.DB) error {
	feedID := mockFeedID()
	base := time.Now().UnixNano()
	ctx := context.Background()

	for i, item := range mockFeedItems {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("mock seed: encode item %q: %w", item.ID, err)
		}
		if err := db.Queries().UpsertFeedItem(ctx, pipelinedb.UpsertFeedItemParams{
			FeedID:    feedID,
			ItemID:    item.ID,
			Payload:   payload,
			UpdatedAt: base - int64(i),
			Unread:    boolToInt64(item.Unread),
		}); err != nil {
			return fmt.Errorf("mock seed: upsert item %q: %w", item.ID, err)
		}
	}
	return nil
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// seedMockFeedItemsOrWarn calls seedMockFeedItems and logs (rather than
// fatals) on failure: an unseeded mock sidebar degrades to empty rather than
// blocking the whole app from starting, matching the best-effort posture
// buildActionStore/buildFlowsStore already use for their own optional setup
// steps in main.go.
func seedMockFeedItemsOrWarn(db *pipelinedb.DB, logger zerolog.Logger) {
	if err := seedMockFeedItems(db); err != nil {
		logger.Warn().Err(err).Msg("mock feed seed failed")
	}
}
