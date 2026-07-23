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

// MockFlowID, MockSourceNodeID, and MockFeedNodeID identify the fixture graph
// in desktop/e2e/fixtures/flows/frontend-triage.yaml. Tests assert these IDs
// against the fixture so its graph configuration cannot drift unnoticed.
const (
	MockFlowID       = "frontend-triage"
	MockSourceNodeID = "gh-source"
	MockFeedNodeID   = "notifications-inbox"
)

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
		Author: "hayden", Unread: true, Reason: "mention", Labels: []string{"feature", "mvp"},
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
		Author: "hayden", Unread: false, Labels: []string{"feature"},
		Branch: "feat/1177-cross-repo-query",
		Body:   "Support GitHub search-style cross-repo queries as a feed source, e.g. \"PRs assigned to me across the org\", saveable as a workspace source.",
		Prompt: "Implement a cross-repo query feed source using GitHub search syntax, saveable into a workspace.",
		URL:    "https://github.com/hive/desktop/issues/1177",
	},
}

// seedMockInboxItems writes deterministic inbox rows directly rather than
// using the ingestion transaction. This is intentionally fixture-only.
func seedMockInboxItems(db *pipelinedb.DB) error {
	base := time.Now().UnixMilli()
	ctx := context.Background()
	sourceTopic := "source:" + MockFlowID + "/" + MockSourceNodeID
	snapshot := make([]pipelinedb.SnapshotItem, 0, len(mockInboxItems))

	for i, item := range mockInboxItems {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("mock seed: encode item %q: %w", item.ID, err)
		}
		row, err := db.Queries().InsertInboxItem(ctx, pipelinedb.InsertInboxItemParams{
			ProfileID:   MockFlowID,
			SourceKind:  "github",
			SourceScope: "",
			ExternalID:  item.ID,
			Title:       item.Title,
			Url:         item.URL,
			Payload:     payload,
			Unread:      boolToInt64(item.Unread),
			Lifecycle:   "active",
			FirstSeenAt: base - int64(i),
			LastEventAt: base - int64(i),
		})
		if err != nil {
			return fmt.Errorf("mock seed: insert item %q: %w", item.ID, err)
		}
		if err := db.Queries().UpsertFeedMembershipClaim(ctx, pipelinedb.UpsertFeedMembershipClaimParams{
			ProfileID: MockFlowID, FeedID: MockFlowID + "/" + MockFeedNodeID, ItemID: row.ID, SourceID: sourceTopic,
		}); err != nil {
			return fmt.Errorf("mock seed: claim item %q: %w", item.ID, err)
		}
		snapshot = append(snapshot, pipelinedb.SnapshotItem{Key: item.ID, Payload: payload})
	}
	if _, err := db.AppendSnapshot(ctx, sourceTopic, "github", "", snapshot); err != nil {
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

func seedMockInboxItemsOrWarn(db *pipelinedb.DB, logger zerolog.Logger) {
	if err := seedMockInboxItems(db); err != nil {
		logger.Warn().Err(err).Msg("mock inbox seed failed")
	}
}
