package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
)

// MockFlowID, MockSourceNodeID, and MockFeedNodeID identify the fixture graph
// in desktop/e2e/fixtures/flows/frontend-triage.yaml. Tests assert these IDs
// against the fixture so its graph configuration cannot drift unnoticed.
const (
	MockFlowID       = "frontend-triage"
	MockSourceNodeID = "gh-source"
	MockFeedNodeID   = "notifications-inbox"
)

// mockItemAges is how long ago each mockInboxItems entry last saw activity,
// positionally paired with that slice. Ages must stay strictly increasing —
// the fixture's newest-first order is the seeded order — and are spread across
// the feed list's date tiers (today through this/last month, depending on where
// in the month the app runs) so mock mode exercises the date separators instead
// of stacking every row under "Today".
var mockItemAges = []time.Duration{
	45 * time.Minute,
	5 * time.Hour,
	30 * time.Hour,
	3 * 24 * time.Hour,
	9 * 24 * time.Hour,
	23 * 24 * time.Hour,
}

// mockInboxItems is the deterministic fixture set used by desktop e2e.
var mockInboxItems = []feed.Item{
	{
		ID: "pr2841", Kind: "PR", Repo: "hive/core", Num: 2841,
		Title:  "batch_spawn: fix detached tmux env & PATH propagation",
		Author: "lena", Unread: true, Labels: []string{"bug", "batch"},
		Branch: "fix/2841-batch-spawn-env",
		Body:   "Sessions spawned from a GUI context inherit an empty PATH and lose HIVE_* vars, so batch_spawn fails to find the agent binary. Needs a controlled env when there is no controlling terminal.",
		Prompt: "Investigate detached tmux env in batch_spawn; ensure PATH and HIVE_* vars propagate when spawned headless from the desktop app.",
		URL:    "https://github.com/hive/core/pull/2841",
	},
	{
		ID: "iss1190", Kind: "Issue", Repo: "hive/desktop", Num: 1190,
		Title:  "Feed source: mirror GitHub notifications inbox",
		Author: "octocat", Unread: true, Reason: "mention", Labels: []string{"feature", "mvp"},
		Branch: "feat/1190-notifications-feed",
		Body:   "Add a notifications-based feed source that mirrors the user's GitHub inbox, with local read/dismiss state so triage does not touch GitHub until the user acts.",
		Prompt: "Implement a notifications-based feed source mirroring the GitHub inbox, with app-local read/dismiss triage state.",
		URL:    "https://github.com/hive/desktop/issues/1190",
	},
	{
		ID: "pr2838", Kind: "PR", Repo: "hive/desktop", Num: 2838,
		Title:  "OAuth device flow for in-app GitHub auth",
		Author: "koji", Unread: false, Labels: []string{"auth"},
		Branch: "feat/2838-oauth-device-flow",
		Body:   "Adds the full device-flow auth so users can sign in without leaving the app. Open question on GitHub App vs OAuth App registration and where to store the token.",
		Prompt: "Review the OAuth device-flow implementation and validate keychain token storage across platforms.",
		URL:    "https://github.com/hive/desktop/pull/2838",
	},
	{
		ID: "iss1204", Kind: "Issue", Repo: "hive/desktop", Num: 1204,
		Title:  "Composable view contract for feed / task / doc surfaces",
		Author: "mira", Unread: true, Labels: []string{"arch"},
		Branch: "feat/1204-composable-views",
		Body:   "Define a self-contained component contract for feed, task, and doc views so a designer-led layout system can be dropped in later without rewrites.",
		Prompt: "Draft a composable, self-contained view interface covering the feed, task list, and doc viewer surfaces.",
		URL:    "https://github.com/hive/desktop/issues/1204",
	},
	{
		ID: "pr2830", Kind: "PR", Repo: "hive/core", Num: 2830,
		Title:  "Keychain-backed token storage",
		Author: "sam", Unread: false, Labels: []string{"security"},
		Branch: "feat/2830-keychain-tokens",
		Body:   "Store GitHub tokens in the OS keychain instead of a plaintext config file, with a fallback for headless CI environments.",
		Prompt: "Review cross-platform keychain token storage and the headless fallback path.",
		URL:    "https://github.com/hive/core/pull/2830",
	},
	{
		ID: "iss1177", Kind: "Issue", Repo: "hive/desktop", Num: 1177,
		Title:  "Cross-repo query: PRs assigned to me across the org",
		Author: "octocat", Unread: false, Labels: []string{"feature"},
		Branch: "feat/1177-cross-repo-query",
		Body:   "Support GitHub search-style cross-repo queries as a feed source, e.g. \"PRs assigned to me across the org\", saveable as a workspace source.",
		Prompt: "Implement a cross-repo query feed source using GitHub search syntax, saveable into a workspace.",
		URL:    "https://github.com/hive/desktop/issues/1177",
	},
}

// seedMockInboxItems writes deterministic inbox rows directly rather than
// using the ingestion transaction. This is intentionally fixture-only.
func seedMockInboxItems(ctx context.Context, db *queries.DB, logs *stores.EventLogStore) error {
	return db.WithinTx(ctx, func(ctx context.Context, tx *queries.DB) error {
		return seedMockInboxItemsTx(ctx, tx, logs)
	})
}

// seedMockInboxItemsTx is the transaction-scoped seed body. Startup seeding
// wraps it in its own transaction; the /_e2e/reset harness reuses it inside
// ResetAllState's wipe transaction so the delete and reseed commit atomically.
// logs.AppendSnapshot joins the same transaction through the ambient context
// WithinTx set up, regardless of which *queries.DB backs it.
func seedMockInboxItemsTx(ctx context.Context, db *queries.DB, logs *stores.EventLogStore) error {
	if len(mockItemAges) != len(mockInboxItems) {
		return fmt.Errorf("mock seed: %d ages for %d items", len(mockItemAges), len(mockInboxItems))
	}
	base := time.Now().UnixMilli()
	q := db.Ctx(ctx).Queries
	sourceTopic := "source:" + MockFlowID + "/" + MockSourceNodeID
	snapshot := make([]models.SnapshotItem, 0, len(mockInboxItems))

	for i, item := range mockInboxItems {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("mock seed: encode item %q: %w", item.ID, err)
		}
		seenAt := base - mockItemAges[i].Milliseconds()
		row, err := q.InsertInboxItem(ctx, queries.InsertInboxItemParams{
			ProfileID:   MockFlowID,
			SourceKind:  "github",
			SourceScope: "",
			ExternalID:  item.ID,
			Title:       item.Title,
			Url:         item.URL,
			Payload:     payload,
			Unread:      boolToInt64(item.Unread),
			Lifecycle:   "active",
			FirstSeenAt: seenAt,
			LastEventAt: seenAt,
		})
		if err != nil {
			return fmt.Errorf("mock seed: insert item %q: %w", item.ID, err)
		}
		if err := q.UpsertFeedMembershipClaim(ctx, queries.UpsertFeedMembershipClaimParams{
			ProfileID: MockFlowID, FeedID: MockFlowID + "/" + MockFeedNodeID, ItemID: row.ID, SourceID: sourceTopic,
		}); err != nil {
			return fmt.Errorf("mock seed: claim item %q: %w", item.ID, err)
		}
		snapshot = append(snapshot, models.SnapshotItem{Key: item.ID, Payload: payload})
	}
	if _, err := logs.AppendSnapshot(ctx, sourceTopic, "github", "", snapshot); err != nil {
		return fmt.Errorf("mock seed: append source snapshot: %w", err)
	}
	return nil
}

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

type mockSeeder struct {
	db   *queries.DB
	logs *stores.EventLogStore
}

func (s mockSeeder) Seed(ctx context.Context) error {
	return seedMockInboxItemsTx(ctx, s.db, s.logs)
}

func SeedMockInboxItemsOrWarn(ctx context.Context, db *queries.DB, logs *stores.EventLogStore, logger zerolog.Logger) {
	if err := seedMockInboxItems(ctx, db, logs); err != nil {
		logger.Warn().Err(err).Msg("mock inbox seed failed")
	}
}
