package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/github/feed"
)

// sourceToCommitSmokePath is available only to the dedicated server-build e2e
// fixture (HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE=pipeline). It is deliberately not a general test
// data API: POST always appends this fixed source fixture, while GET reports
// only persisted node runs during this temporary Phase-1 degraded harness.
const sourceToCommitSmokePath = "/_e2e/source-to-commit"

const (
	sourceToCommitSmokeFlowID   = "source-to-commit"
	sourceToCommitSmokeSourceID = "fixture-source"
	sourceToCommitSmokeFeedID   = "source-to-commit/smoke-feed"
	sourceToCommitSmokeNotifyID = "source-to-commit/smoke-notify"
)

var sourceToCommitSmokeItems = []feed.Item{
	{
		ID: "smoke-pr", Kind: "PR", Repo: "hive/e2e", Num: 101,
		Title: "Source-to-commit smoke PR", Author: "smoke", Unread: true,
		Labels: []string{"e2e"}, Branch: "test/source-to-commit",
		Body:   "Fixture item appended by a source and committed through the flow engine.",
		Prompt: "Verify the source-to-commit desktop smoke path.",
		URL:    "https://example.invalid/hive/e2e/pull/101",
	},
	{
		ID: "smoke-issue", Kind: "Issue", Repo: "hive/e2e", Num: 102,
		Title: "Source-to-commit smoke issue", Author: "smoke", Unread: true,
		Labels: []string{"e2e"}, Branch: "test/source-to-commit",
		Body:   "Second fixture item proves one batch commits multiple outputs.",
		Prompt: "Verify one frontend batch processes multiple outputs.",
		URL:    "https://example.invalid/hive/e2e/issues/102",
	},
}

type sourceToCommitSmokeState struct {
	Claims   []stores.InboxItem     `json:"claims"`
	NodeRuns []stores.NodeRunRecord `json:"nodeRuns"`
	// NotifyCommands counts the notify node's enqueued output commands: the
	// observable proof that the KV-backed dedup branch fired once per item
	// and stayed quiet on a changed re-observation.
	NotifyCommands int `json:"notifyCommands"`
}

// sourceToCommitSmokeClassifier is the deliberately small source-side
// classifier used by this fixture. IngestObservation remains the production
// source boundary: it creates the inbox identity and appends the event the
// flow engine consumes. The smoke test therefore cannot pass from a
// pre-seeded claim.
type sourceToCommitSmokeClassifier struct{}

func (sourceToCommitSmokeClassifier) Classify(previous *models.Observation, current models.Observation) models.Classification {
	if previous == nil {
		return models.Classification{Kind: "observed", Transition: models.TransitionNone, Attention: models.AttentionActivity, Lifecycle: models.LifecycleActive, Summary: current.Title}
	}
	return models.Classification{Kind: "updated", Transition: models.TransitionNone, Attention: models.AttentionTrivial, Lifecycle: models.LifecycleActive, Summary: current.Title}
}

// sourceToCommitSmokeMiddleware is a narrow, mock-only harness around the
// real server build. This middleware only supplies deterministic source input
// and reads the persisted node runs back for Playwright; everything between
// is production — the flow engine wakes, routes the batch through the fixture
// graph, and commits.
//
// onAppended announces that the event log grew and wakes the engine, exactly
// as the producer does. It is supplied rather than called directly so this
// package does not have to import the adapter that mounts it.
func sourceToCommitSmokeMiddleware(db *queries.DB, st *stores.Stores, mock string, onAppended func(nextOffset int64)) application.Middleware {
	return func(next http.Handler) http.Handler {
		if mock != "pipeline" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != sourceToCommitSmokePath {
				next.ServeHTTP(w, r)
				return
			}

			switch r.Method {
			case http.MethodPost:
				if err := appendSourceToCommitSmokeItems(r.Context(), st, r.URL.Query().Get("rev"), onAppended); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]int{"appended": len(sourceToCommitSmokeItems)})
			case http.MethodGet:
				state, err := readSourceToCommitSmokeState(r.Context(), db, st)
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

// appendSourceToCommitSmokeItems ingests the fixture items. A non-empty rev
// beyond "1" varies each title, so the re-observation is a genuine change
// that appends and routes — the case KV dedup exists to suppress.
func appendSourceToCommitSmokeItems(ctx context.Context, st *stores.Stores, rev string, onAppended func(nextOffset int64)) error {
	var lastOffset int64
	for _, item := range sourceToCommitSmokeItems {
		if rev != "" && rev != "1" {
			item.Title += " (rev " + rev + ")"
		}
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("encode smoke fixture item %q: %w", item.ID, err)
		}
		// IngestObservation is the production source boundary. It creates the
		// inbox identity and appends the event log record; the graph still has to
		// traverse all nodes and Commit has to create the feed claim.
		result, err := st.InboxItems.IngestObservation(ctx, sourceToCommitSmokeClassifier{}, stores.IngestObservationParams{
			ProfileID: sourceToCommitSmokeFlowID,
			Topic:     "source:" + sourceToCommitSmokeFlowID + "/" + sourceToCommitSmokeSourceID,
			Policy:    models.ResurfacePolicyStateChanges,
			Current: models.Observation{
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
	if lastOffset > 0 && onAppended != nil {
		onAppended(lastOffset)
	}
	return nil
}

func readSourceToCommitSmokeState(ctx context.Context, db *queries.DB, st *stores.Stores) (sourceToCommitSmokeState, error) {
	claims, err := st.InboxItems.ListByFeed(ctx, sourceToCommitSmokeFlowID, sourceToCommitSmokeFeedID, 100)
	if err != nil {
		return sourceToCommitSmokeState{}, fmt.Errorf("read smoke claims: %w", err)
	}
	runs, err := st.NodeRuns.List(ctx, sourceToCommitSmokeFlowID, 100)
	if err != nil {
		return sourceToCommitSmokeState{}, fmt.Errorf("read smoke node runs: %w", err)
	}
	var notifyCommands int
	if err := db.Conn().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM output_command WHERE action_id = ?`,
		models.NotifyActionID(sourceToCommitSmokeNotifyID),
	).Scan(&notifyCommands); err != nil {
		return sourceToCommitSmokeState{}, fmt.Errorf("count smoke notify commands: %w", err)
	}
	return sourceToCommitSmokeState{Claims: claims, NodeRuns: runs, NotifyCommands: notifyCommands}, nil
}
