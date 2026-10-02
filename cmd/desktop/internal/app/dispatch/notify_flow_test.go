package dispatch

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
)

type notifyActivityRecorder struct {
	events []activity.Event
}

func (r *notifyActivityRecorder) Record(_ context.Context, event activity.Event) {
	r.events = append(r.events, event)
}

// The whole notify path, end to end through the production seams a flow
// actually uses: the graph runtime's commit, the durable queue, the worker's
// action resolution against the live flow set, and the executor. Only the OS
// itself is faked.
func TestNotifyTerminal_DeliversThroughTheWorker(t *testing.T) {
	ctx := t.Context()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	item, err := stores.NewSeed(db).InboxItem(ctx, stores.InboxItem{
		ProfileID: "triage", SourceKind: "github", SourceScope: "src", ExternalID: "acme/api#12",
		Payload: []byte(`{"repo":"acme/api","title":"Fix the flake","url":"https://github.com/acme/api/pull/12"}`), Lifecycle: "active",
	})
	require.NoError(t, err)

	flows := flowListerTest{flows: []flow.Flow{{
		ID:      "triage",
		Enabled: true,
		Nodes: []flow.Node{{ID: "tell-me", Type: "notify", Name: "Tell me", Config: &flow.NotifyConfig{
			Title: "{{ .Payload.repo }} needs review", Body: "{{ .Payload.title }}",
		}}},
	}}}
	notifier := &notifierTest{}
	dispatcher := NewDispatcher(map[string]Executor{
		ActionTypeNotify: NewNotifyExecutor(notifier, openGate(), stores.New(db, stores.Options{}).InboxItems, zerolog.Nop()),
	})
	worker := NewWorker(testOutputCommands(db), NewFlowActions(flows, actionListerTest{}), dispatcher, DefaultOutputWorkerInterval, zerolog.Nop())
	activityRecorder := &notifyActivityRecorder{}
	worker.SetRecorder(activityRecorder)

	// What the graph runtime (cmd/desktop/internal/app/runtime) commits for a message
	// reaching a notify terminal.
	commit := func(offset int64, occurrence string) {
		t.Helper()
		require.NoError(t, stores.New(db, stores.Options{}).EventLog.Commit(ctx, models.CommitBatch{
			Consumer: "triage", UpToOffset: offset,
			Outputs: []models.Output{{
				Sink:          models.Sink{Kind: models.SinkKindNotify, TargetID: "triage/tell-me"},
				Key:           "acme/api#12",
				OccurrenceKey: occurrence,
				SourceKind:    "github",
				SourceScope:   "src",
				SourceTopic:   "source:triage/src",
				Payload:       []byte(`{"repo":"acme/api","title":"Fix the flake","url":"https://github.com/acme/api/pull/12"}`),
			}},
		}))
	}

	commit(1, "acme/api#12:open:100:comment")
	worker.Tick(ctx)

	require.Len(t, notifier.sent, 1)
	assert.Equal(t, "acme/api needs review", notifier.sent[0].Title)
	assert.Equal(t, "Fix the flake", notifier.sent[0].Body)
	assert.Equal(t, map[string]any{"profileId": "triage", "itemId": item.ID}, notifier.sent[0].Data,
		"the notification must carry the item a click should reveal")
	require.Len(t, activityRecorder.events, 1)
	assert.Equal(t, "rule notify:triage/tell-me · acme/api#12 · no confirmation required", activityRecorder.events[0].Body)
	assert.Equal(t, map[string]string{
		activity.MetadataLinkURL:             "https://github.com/acme/api/pull/12",
		activity.MetadataLinkItemProfileID:   "triage",
		activity.MetadataLinkItemSourceKind:  "github",
		activity.MetadataLinkItemSourceScope: "src",
		activity.MetadataLinkItemExternalID:  "acme/api#12",
	}, activityRecorder.events[0].Metadata)

	// The command is terminal, so a second tick cannot re-fire it.
	worker.Tick(ctx)
	assert.Len(t, notifier.sent, 1)

	// Nor can the same occurrence arriving again in a later batch.
	commit(2, "acme/api#12:open:100:comment")
	worker.Tick(ctx)
	assert.Len(t, notifier.sent, 1)
}

// A notify node the author deleted (or renamed) leaves its queued commands
// unresolvable. They must fail visibly rather than hang in the queue.
func TestNotifyTerminal_DeletedNodeFailsItsQueuedCommand(t *testing.T) {
	ctx := t.Context()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	notifier := &notifierTest{}
	dispatcher := NewDispatcher(map[string]Executor{
		ActionTypeNotify: NewNotifyExecutor(notifier, openGate(), stores.New(db, stores.Options{}).InboxItems, zerolog.Nop()),
	})
	worker := NewWorker(testOutputCommands(db), NewFlowActions(flowListerTest{}, actionListerTest{}), dispatcher, DefaultOutputWorkerInterval, zerolog.Nop())

	require.NoError(t, stores.New(db, stores.Options{}).EventLog.Commit(ctx, models.CommitBatch{
		Consumer: "triage", UpToOffset: 1,
		Outputs: []models.Output{{
			Sink:          models.Sink{Kind: models.SinkKindNotify, TargetID: "triage/deleted"},
			Key:           "acme/api#12",
			OccurrenceKey: "occ",
			Payload:       []byte(`{}`),
		}},
	}))
	worker.Tick(ctx)

	assert.Empty(t, notifier.sent)
	var status, lastError string
	require.NoError(t, db.Conn().QueryRowContext(ctx,
		`SELECT status, COALESCE(last_error, '') FROM output_command`).Scan(&status, &lastError))
	assert.Equal(t, "pending", status, "an unresolvable command is retried before it is given up on")
	assert.Contains(t, lastError, "unknown action")
}
