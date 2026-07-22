package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/feed"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// sourceToCommitSmokePath is available only to the dedicated server-build e2e
// fixture (HIVE_DESKTOP_MOCK=pipeline). It is deliberately not a general test
// data API: POST always appends this fixed source fixture, while GET reports
// only persisted node runs during this temporary Phase-1 degraded harness.
const sourceToCommitSmokePath = "/_e2e/source-to-commit"

const (
	sourceToCommitSmokeFlowID   = "source-to-commit"
	sourceToCommitSmokeSourceID = "fixture-source"
	sourceToCommitSmokeFeedID   = "source-to-commit/smoke-feed"
)

var sourceToCommitSmokeItems = []feed.Item{
	{
		ID: "smoke-pr", Kind: "PR", Repo: "hive/e2e", Num: 101,
		Title: "Source-to-commit smoke PR", Author: "smoke", Unread: true,
		Labels: []string{"e2e"}, Branch: "test/source-to-commit",
		Body:   "Fixture item appended by Go and committed through the browser graph.",
		Prompt: "Verify the source-to-commit desktop smoke path.",
		URL:    "https://example.invalid/hive/e2e/pull/101",
	},
	{
		ID: "smoke-issue", Kind: "Issue", Repo: "hive/e2e", Num: 102,
		Title: "Source-to-commit smoke issue", Author: "smoke", Unread: true,
		Labels: []string{"e2e"}, Branch: "test/source-to-commit",
		Body:   "Second fixture item proves one frontend batch commits multiple outputs.",
		Prompt: "Verify one frontend batch processes multiple outputs.",
		URL:    "https://example.invalid/hive/e2e/issues/102",
	},
}

type sourceToCommitSmokeState struct {
	Claims   []pipelinedb.InboxItemView `json:"claims"`
	NodeRuns []pipelinedb.NodeRunRecord `json:"nodeRuns"`
}

// sourceToCommitSmokeClassifier is the deliberately small source-side
// classifier used by this fixture. IngestObservation remains the production
// source boundary: it creates the inbox identity and appends the event that
// the browser graph consumes. The smoke test therefore cannot pass from a
// pre-seeded claim.
type sourceToCommitSmokeClassifier struct{}

func (sourceToCommitSmokeClassifier) Classify(previous *pipelinedb.Observation, current pipelinedb.Observation) pipelinedb.Classification {
	if previous == nil {
		return pipelinedb.Classification{Kind: "observed", Transition: pipelinedb.TransitionNone, Attention: pipelinedb.AttentionActivity, Lifecycle: pipelinedb.LifecycleActive, Summary: current.Title}
	}
	return pipelinedb.Classification{Kind: "updated", Transition: pipelinedb.TransitionNone, Attention: pipelinedb.AttentionTrivial, Lifecycle: pipelinedb.LifecycleActive, Summary: current.Title}
}

// sourceToCommitSmokeMiddleware is a narrow, mock-only harness around the
// real server build. The app still receives its messages via the normal Wails
// event, executes the production TS graph and Worker, then calls
// PipelineService.Commit; this middleware merely supplies deterministic Go
// source input and reads the persisted node runs back for Playwright.
func sourceToCommitSmokeMiddleware(db *pipelinedb.DB) application.Middleware {
	return func(next http.Handler) http.Handler {
		if desktop.MockMode() != "pipeline" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sourceToCommitSmokePath {
				next.ServeHTTP(w, r)
				return
			}

			switch r.Method {
			case http.MethodPost:
				if err := appendSourceToCommitSmokeItems(r.Context(), db); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]int{"appended": len(sourceToCommitSmokeItems)})
			case http.MethodGet:
				state, err := readSourceToCommitSmokeState(r.Context(), db)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(state)
			default:
				w.Header().Set("Allow", "GET, POST")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}
}

func appendSourceToCommitSmokeItems(ctx context.Context, db *pipelinedb.DB) error {
	var lastOffset int64
	for _, item := range sourceToCommitSmokeItems {
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("encode smoke fixture item %q: %w", item.ID, err)
		}
		// IngestObservation is the production source boundary. It creates the
		// inbox identity and appends the event log record; the graph still has to
		// traverse all nodes and Commit has to create the feed claim.
		result, err := db.IngestObservation(ctx, sourceToCommitSmokeClassifier{}, pipelinedb.IngestObservationParams{
			ProfileID: sourceToCommitSmokeFlowID,
			Topic:     "source:" + sourceToCommitSmokeFlowID + "/" + sourceToCommitSmokeSourceID,
			Policy:    pipelinedb.ResurfacePolicyStateChanges,
			Current: pipelinedb.Observation{
				ExternalID: item.ID, Title: item.Title, URL: item.URL,
				SourceKind: "github", SourceScope: sourceToCommitSmokeSourceID,
				ObservedAt: time.Now().UnixMilli(), Payload: payload,
			},
		})
		if err != nil {
			return fmt.Errorf("ingest smoke fixture item %q: %w", item.ID, err)
		}
		if result.Wrote {
			lastOffset = result.Offset
		}
	}
	if lastOffset > 0 {
		emitLogAppended(lastOffset)
	}
	return nil
}

func readSourceToCommitSmokeState(ctx context.Context, db *pipelinedb.DB) (sourceToCommitSmokeState, error) {
	claims, err := db.ListInboxItemsByFeed(ctx, sourceToCommitSmokeFlowID, sourceToCommitSmokeFeedID, 100)
	if err != nil {
		return sourceToCommitSmokeState{}, fmt.Errorf("read smoke claims: %w", err)
	}
	runs, err := db.NodeRuns(ctx, sourceToCommitSmokeFlowID, 100)
	if err != nil {
		return sourceToCommitSmokeState{}, fmt.Errorf("read smoke node runs: %w", err)
	}
	return sourceToCommitSmokeState{Claims: claims, NodeRuns: runs}, nil
}
