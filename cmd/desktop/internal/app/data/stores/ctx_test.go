package stores

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type ctxFixture struct {
	itemID      int64
	commandID   int64
	agentSessID int64
}

func seedCtxFixture(t *testing.T, st *Stores, db *queries.DB) ctxFixture {
	t.Helper()
	ctx := t.Context()

	item, err := db.InsertInboxItem(ctx, queries.InsertInboxItemParams{
		ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "item-1",
		Title: "Item", Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)

	_, err = db.InsertInboxEvent(ctx, queries.InsertInboxEventParams{
		ItemID: item.ID, Kind: "observed", Transition: "none", Attention: "trivial",
		Detail: []byte(`{}`), CreatedAt: time.Now().UnixMilli(),
	})
	require.NoError(t, err)

	require.NoError(t, db.UpsertFeedMembershipClaim(ctx, queries.UpsertFeedMembershipClaimParams{
		ProfileID: "p", FeedID: "p/feed", ItemID: item.ID, SourceID: "source-a",
	}))

	require.NoError(t, db.LinkItemSession(ctx, queries.LinkItemSessionParams{
		SessionID: "sess-1", ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "item-1",
		CreatedAt: time.Now().UnixMilli(),
	}))

	agentSess, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "demo", Name: "chat", Agent: "claude"})
	require.NoError(t, err)

	require.NoError(t, db.UpsertNodeKV(ctx, queries.UpsertNodeKVParams{
		FlowID: "flow-1", NodeID: "node-a", Scope: KVScopeNode, Key: "k", Value: "v", UpdatedAt: time.Now().UnixMilli(),
	}))

	require.NoError(t, db.InsertNodeRun(ctx, queries.InsertNodeRunParams{
		FlowID: "flow-1", NodeID: "node-a", Ok: 1, EndedAt: time.Now().UnixMilli(),
	}))

	require.NoError(t, db.UpsertWebhookCapture(ctx, queries.UpsertWebhookCaptureParams{
		Topic: "source:flow-1/hook", ReceivedAt: time.Now().UnixMilli(), Body: []byte(`{}`),
	}))

	require.NoError(t, db.UpsertSourceHead(ctx, queries.UpsertSourceHeadParams{
		Topic: "source:flow-1/a", Key: "item-1", Payload: []byte(`{}`),
	}))

	_, err = db.AppendActivityEvent(ctx, queries.AppendActivityEventParams{
		CreatedAt: time.Now().UnixMilli(), Category: "action", Severity: "info", Title: "seed", Source: "test",
	})
	require.NoError(t, err)

	_, err = db.InsertJob(ctx, queries.InsertJobParams{
		CreatedAt: time.Now().UnixMilli(), UpdatedAt: time.Now().UnixMilli(), Status: "queued", Label: "seed",
	})
	require.NoError(t, err)

	command, created, err := st.OutputCommands.Confirm(ctx, "action-a", "item-1", []byte(`{}`), models.ItemRef{
		ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "item-1",
	})
	require.NoError(t, err)
	require.True(t, created)

	return ctxFixture{itemID: item.ID, commandID: command.ID, agentSessID: agentSess.ID}
}

// Cap the pool at one connection so a store method that escapes the ambient
// transaction blocks until its deadline instead of using another connection.
func TestEveryStoreMethodJoinsTheAmbientTransaction(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.OpenOptions{MaxOpenConns: 1, MaxIdleConns: 1, BusyTimeout: 5000})
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	st := New(db, Options{})
	fx := seedCtxFixture(t, st, db)

	type step struct {
		name string
		call func(ctx context.Context) error
	}

	steps := []step{
		{"ActivityEventStore.Append", func(ctx context.Context) error {
			_, err := st.ActivityEvents.Append(ctx, ActivityEventCreate{Category: "action", Severity: "info", Title: "t", Source: "test"})
			return err
		}},
		{"ActivityEventStore.List", func(ctx context.Context) error {
			_, err := st.ActivityEvents.List(ctx, 0, 10)
			return err
		}},

		{"AgentSessionStore.List", func(ctx context.Context) error {
			_, err := st.AgentSessions.List(ctx, "demo")
			return err
		}},
		{"AgentSessionStore.ListAll", func(ctx context.Context) error {
			_, err := st.AgentSessions.ListAll(ctx)
			return err
		}},
		{"AgentSessionStore.Get", func(ctx context.Context) error {
			_, err := st.AgentSessions.Get(ctx, fx.agentSessID)
			return err
		}},
		{"AgentSessionStore.Create", func(ctx context.Context) error {
			_, err := st.AgentSessions.Create(ctx, AgentSessionCreate{Workspace: "demo", Name: "n", Agent: "claude"})
			return err
		}},
		{"AgentSessionStore.Touch", func(ctx context.Context) error {
			return st.AgentSessions.Touch(ctx, fx.agentSessID, time.Now().UnixMilli())
		}},
		{"AgentSessionStore.SetAgentID", func(ctx context.Context) error {
			return st.AgentSessions.SetAgentID(ctx, fx.agentSessID, "agent-sess-1")
		}},
		{"AgentSessionStore.Rename", func(ctx context.Context) error {
			return st.AgentSessions.Rename(ctx, fx.agentSessID, "renamed")
		}},
		{"AgentSessionStore.DeleteByWorkspace", func(ctx context.Context) error {
			return st.AgentSessions.DeleteByWorkspace(ctx, "no-such-workspace")
		}},
		{"AgentSessionStore.Delete", func(ctx context.Context) error {
			return st.AgentSessions.Delete(ctx, fx.agentSessID)
		}},

		{"EventLogStore.Append", func(ctx context.Context) error {
			_, err := st.EventLog.Append(ctx, "source:flow-1/a", "k", []byte(`{}`))
			return err
		}},
		{"EventLogStore.AppendSnapshot", func(ctx context.Context) error {
			_, err := st.EventLog.AppendSnapshot(ctx, "source:flow-1/a", "github", "s", nil)
			return err
		}},
		{"EventLogStore.ReadFrom", func(ctx context.Context) error {
			_, _, err := st.EventLog.ReadFrom(ctx, 0, 10)
			return err
		}},
		{"EventLogStore.ReadForConsumer", func(ctx context.Context) error {
			_, err := st.EventLog.ReadForConsumer(ctx, "flow-1", 10)
			return err
		}},
		{"EventLogStore.ConsumerOffset", func(ctx context.Context) error {
			_, err := st.EventLog.ConsumerOffset(ctx, "flow-1")
			return err
		}},
		{"EventLogStore.TailOffset", func(ctx context.Context) error {
			_, err := st.EventLog.TailOffset(ctx)
			return err
		}},
		{"EventLogStore.ListLatestSnapshots", func(ctx context.Context) error {
			_, err := st.EventLog.ListLatestSnapshots(ctx, "flow-1", 0)
			return err
		}},
		{"EventLogStore.DeleteByTopicPrefix", func(ctx context.Context) error {
			return st.EventLog.DeleteByTopicPrefix(ctx, "source:no-such-flow/")
		}},
		{"EventLogStore.DeleteConsumerOffset", func(ctx context.Context) error {
			return st.EventLog.DeleteConsumerOffset(ctx, "no-such-consumer")
		}},
		{"EventLogStore.AppendObservation", func(ctx context.Context) error {
			_, err := st.EventLog.AppendObservation(ctx, "source:flow-1/ctx", "k-ctx", []byte(`{}`), "github", "s", "occ-ctx", time.Now().UnixMilli())
			return err
		}},
		{"EventLogStore.BackfillOccurrenceKey", func(ctx context.Context) error {
			offset, err := st.EventLog.AppendObservation(ctx, "source:flow-1/ctx2", "k-ctx2", []byte(`{}`), "github", "s", "", time.Now().UnixMilli())
			if err != nil {
				return err
			}
			return st.EventLog.BackfillOccurrenceKey(ctx, offset, "backfilled")
		}},
		{"EventLogStore.Commit", func(ctx context.Context) error {
			return st.EventLog.Commit(ctx, models.CommitBatch{Consumer: "no-such-consumer", UpToOffset: 0})
		}},
		{"EventLogStore.ActivateReplay", func(ctx context.Context) error {
			return st.EventLog.ActivateReplay(ctx, "no-such-profile", 0, nil, nil, nil, nil)
		}},

		{"FeedClaimStore.Upsert", func(ctx context.Context) error {
			return st.FeedClaims.Upsert(ctx, models.FeedClaim{ProfileID: "p", FeedID: "p/other", ItemID: fx.itemID, SourceID: "source-b"})
		}},
		{"FeedClaimStore.DeleteNotInSnapshot", func(ctx context.Context) error {
			return st.FeedClaims.DeleteNotInSnapshot(ctx, "p/other", "source-b", []int64{fx.itemID})
		}},
		{"FeedClaimStore.DeleteForSourceAll", func(ctx context.Context) error {
			return st.FeedClaims.DeleteForSourceAll(ctx, "p/no-such-feed", "source-z")
		}},
		{"FeedClaimStore.DeleteForFeeds", func(ctx context.Context) error {
			return st.FeedClaims.DeleteForFeeds(ctx, "no-such-profile", []string{"p/feed"})
		}},
		{"FeedClaimStore.DeleteForRemovedSources", func(ctx context.Context) error {
			return st.FeedClaims.DeleteForRemovedSources(ctx, "no-such-profile", []string{"source-a"})
		}},
		{"FeedClaimStore.DeleteUnarchivedByProfile", func(ctx context.Context) error {
			return st.FeedClaims.DeleteUnarchivedByProfile(ctx, "no-such-profile")
		}},

		{"InboxItemStore.ListByFeed", func(ctx context.Context) error {
			_, err := st.InboxItems.ListByFeed(ctx, "p", "p/feed", 10)
			return err
		}},
		{"InboxItemStore.ListArchivedByFeed", func(ctx context.Context) error {
			_, err := st.InboxItems.ListArchivedByFeed(ctx, "p", "p/feed", 10)
			return err
		}},
		{"InboxItemStore.ListTrash", func(ctx context.Context) error {
			_, err := st.InboxItems.ListTrash(ctx, "p", 10)
			return err
		}},
		{"InboxItemStore.ListAll", func(ctx context.Context) error {
			_, err := st.InboxItems.ListAll(ctx, "p", 10)
			return err
		}},
		{"InboxItemStore.ListUnarchived", func(ctx context.Context) error {
			_, err := st.InboxItems.ListUnarchived(ctx, "p")
			return err
		}},
		{"InboxItemStore.ListUnarchivedBySource", func(ctx context.Context) error {
			_, err := st.InboxItems.ListUnarchivedBySource(ctx, "p", "github", "s")
			return err
		}},
		{"InboxItemStore.FindByExternalID", func(ctx context.Context) error {
			_, err := st.InboxItems.FindByExternalID(ctx, "p", "item-1")
			return err
		}},
		{"InboxItemStore.GetByID", func(ctx context.Context) error {
			_, err := st.InboxItems.GetByID(ctx, fx.itemID)
			return err
		}},
		{"InboxItemStore.RefByID", func(ctx context.Context) error {
			_, err := st.InboxItems.RefByID(ctx, fx.itemID)
			return err
		}},
		{"InboxItemStore.ResolveScoped", func(ctx context.Context) error {
			_, err := st.InboxItems.ResolveScoped(ctx, "p", "github", "s", "item-1")
			return err
		}},
		{"InboxItemStore.ResolveScopedID", func(ctx context.Context) error {
			_, err := st.InboxItems.ResolveScopedID(ctx, "p", "github", "s", "item-1")
			return err
		}},
		{"InboxItemStore.IDByExternalID", func(ctx context.Context) error {
			_, err := st.InboxItems.IDByExternalID(ctx, "p", "github", "s", "item-1")
			return err
		}},
		{"InboxItemStore.FeedIDForItem", func(ctx context.Context) error {
			_, err := st.InboxItems.FeedIDForItem(ctx, "p", fx.itemID)
			return err
		}},
		{"InboxItemStore.FeedIDsForItems", func(ctx context.Context) error {
			_, err := st.InboxItems.FeedIDsForItems(ctx, []int64{fx.itemID})
			return err
		}},
		{"InboxItemStore.Events", func(ctx context.Context) error {
			_, err := st.InboxItems.Events(ctx, fx.itemID, 10)
			return err
		}},
		{"InboxItemStore.FeedCounts", func(ctx context.Context) error {
			_, err := st.InboxItems.FeedCounts(ctx, "p")
			return err
		}},
		{"InboxItemStore.MarkRead", func(ctx context.Context) error {
			_, err := st.InboxItems.MarkRead(ctx, "p", "p/feed")
			return err
		}},
		{"InboxItemStore.SetUnread", func(ctx context.Context) error {
			_, err := st.InboxItems.SetUnread(ctx, fx.itemID, 1, true)
			return err
		}},
		{"InboxItemStore.ToggleArchived", func(ctx context.Context) error {
			_, err := st.InboxItems.ToggleArchived(ctx, fx.itemID, 1)
			return err
		}},
		{"InboxItemStore.ToggleIgnored", func(ctx context.Context) error {
			_, err := st.InboxItems.ToggleIgnored(ctx, fx.itemID, 1)
			return err
		}},
		{"InboxItemStore.DeleteByProfile", func(ctx context.Context) error {
			return st.InboxItems.DeleteByProfile(ctx, "no-such-profile")
		}},
		{"InboxItemStore.GetUnarchivedByID", func(ctx context.Context) error {
			_, err := st.InboxItems.GetUnarchivedByID(ctx, fx.itemID, "p")
			return err
		}},
		{"InboxItemStore.CreateSynthesized", func(ctx context.Context) error {
			_, err := st.InboxItems.CreateSynthesized(ctx, InboxItemSynthesize{
				ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "ctx-synth",
				Payload: []byte(`{}`), Now: time.Now().UnixMilli(),
			})
			return err
		}},
		{"InboxItemStore.IngestObservation", func(ctx context.Context) error {
			_, err := st.InboxItems.IngestObservation(ctx, activityClassifier("ctx-ingest"), IngestObservationParams{
				ProfileID: "p", Topic: "source:p/ctx-ingest",
				Current: models.Observation{ExternalID: "ctx-ingest-item", SourceKind: "github", SourceScope: "s", ObservedAt: 1, Payload: []byte(`{"v":1}`)},
			})
			return err
		}},

		{"ItemSessionStore.Link", func(ctx context.Context) error {
			return st.ItemSessions.Link(ctx, "sess-2", models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "item-1"})
		}},
		{"ItemSessionStore.List", func(ctx context.Context) error {
			_, err := st.ItemSessions.List(ctx, models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "item-1"})
			return err
		}},
		{"ItemSessionStore.Unlink", func(ctx context.Context) error {
			return st.ItemSessions.Unlink(ctx, []string{"sess-2"})
		}},
		{"ItemSessionStore.DeleteByProfile", func(ctx context.Context) error {
			return st.ItemSessions.DeleteByProfile(ctx, "no-such-profile")
		}},

		{"JobStore.Insert", func(ctx context.Context) error {
			_, err := st.Jobs.Insert(ctx, JobCreate{Status: "queued", Label: "l"})
			return err
		}},
		{"JobStore.SetRunning", func(ctx context.Context) error {
			job, err := st.Jobs.Insert(ctx, JobCreate{Status: "queued", Label: "l2"})
			if err != nil {
				return err
			}
			_, err = st.Jobs.SetRunning(ctx, job.ID, "Running", fx.commandID)
			return err
		}},
		{"JobStore.SetStatus", func(ctx context.Context) error {
			job, err := st.Jobs.Insert(ctx, JobCreate{Status: "queued", Label: "l3"})
			if err != nil {
				return err
			}
			_, err = st.Jobs.SetStatus(ctx, job.ID, "done", "Done", "")
			return err
		}},
		{"JobStore.FindRunningByCommand", func(ctx context.Context) error {
			_, _, err := st.Jobs.FindRunningByCommand(ctx, fx.commandID)
			return err
		}},
		{"JobStore.List", func(ctx context.Context) error {
			_, err := st.Jobs.List(ctx, 0, 10)
			return err
		}},
		{"JobStore.ListActive", func(ctx context.Context) error {
			_, err := st.Jobs.ListActive(ctx, 0)
			return err
		}},

		{"NodeKVStore.Get", func(ctx context.Context) error {
			_, _, err := st.NodeKV.Get(ctx, "flow-1", "node-a", "k", time.Now().UnixMilli())
			return err
		}},
		{"NodeKVStore.Keys", func(ctx context.Context) error {
			_, err := st.NodeKV.Keys(ctx, "flow-1", "node-a", "k", time.Now().UnixMilli())
			return err
		}},
		{"NodeKVStore.Set", func(ctx context.Context) error {
			return st.NodeKV.Set(ctx, "flow-1", "node-a", "k2", "v2", 0)
		}},
		{"NodeKVStore.DeleteByFlow", func(ctx context.Context) error {
			return st.NodeKV.DeleteByFlow(ctx, "no-such-flow")
		}},

		{"NodeRunStore.List", func(ctx context.Context) error {
			_, err := st.NodeRuns.List(ctx, "flow-1", 10)
			return err
		}},
		{"NodeRunStore.Insert", func(ctx context.Context) error {
			return st.NodeRuns.Insert(ctx, models.NodeRun{FlowID: "flow-1", NodeID: "node-ctx", OK: true}, time.Now().UnixMilli())
		}},

		{"OutputCommandStore.ListRunnableAfter", func(ctx context.Context) error {
			_, err := st.OutputCommands.ListRunnableAfter(ctx, 0, 10)
			return err
		}},
		{"OutputCommandStore.Enqueue", func(ctx context.Context) error {
			return st.OutputCommands.Enqueue(ctx, "action-ctx", "key-ctx", []byte(`{}`), time.Now().UnixMilli(), models.ItemRef{})
		}},
		{"OutputCommandStore.Confirm", func(ctx context.Context) error {
			_, _, err := st.OutputCommands.Confirm(ctx, "action-b", "item-1", []byte(`{}`), models.ItemRef{ProfileID: "p", SourceKind: "github", SourceScope: "s", ExternalID: "item-1"})
			return err
		}},
		{"OutputCommandStore.Rerun", func(ctx context.Context) error {
			_, err := st.OutputCommands.Rerun(ctx, "no-such-action", "no-such-key", []byte(`{}`), models.ItemRef{})
			// A miss proves the query ran; an escaped call would have timed
			// out on the pool instead.
			if IsNotFound(err) {
				return nil
			}
			return err
		}},
		{"OutputCommandStore.Get", func(ctx context.Context) error {
			_, err := st.OutputCommands.Get(ctx, fx.commandID)
			return err
		}},
		{"OutputCommandStore.MarkDone", func(ctx context.Context) error {
			return st.OutputCommands.MarkDone(ctx, fx.commandID)
		}},
		{"OutputCommandStore.MarkFailed", func(ctx context.Context) error {
			return st.OutputCommands.MarkFailed(ctx, fx.commandID, "boom")
		}},
		{"OutputCommandStore.Retry", func(ctx context.Context) error {
			return st.OutputCommands.Retry(ctx, fx.commandID, "boom")
		}},
		{"OutputCommandStore.CountNonterminalForAction", func(ctx context.Context) error {
			_, err := st.OutputCommands.CountNonterminalForAction(ctx, "action-a")
			return err
		}},

		{"SourceHeadStore.ListActiveKeys", func(ctx context.Context) error {
			_, err := st.SourceHeads.ListActiveKeys(ctx, SourceIdentity{Topic: "source:flow-1/a", ProfileID: "p", SourceKind: "github", SourceScope: "s"})
			return err
		}},
		{"SourceHeadStore.Payload", func(ctx context.Context) error {
			_, err := st.SourceHeads.Payload(ctx, "source:flow-1/a", "item-1")
			return err
		}},
		{"SourceHeadStore.Upsert", func(ctx context.Context) error {
			return st.SourceHeads.Upsert(ctx, "source:flow-1/b", "item-2", []byte(`{}`))
		}},
		{"SourceHeadStore.Delete", func(ctx context.Context) error {
			return st.SourceHeads.Delete(ctx, "source:flow-1/b", "item-2")
		}},
		{"SourceHeadStore.DeleteByTopicPrefix", func(ctx context.Context) error {
			return st.SourceHeads.DeleteByTopicPrefix(ctx, "source:no-such-flow/")
		}},

		{"WebhookCaptureStore.Upsert", func(ctx context.Context) error {
			return st.WebhookCaptures.Upsert(ctx, "source:flow-1/hook2", time.Now().UnixMilli(), []byte(`{}`))
		}},
		{"WebhookCaptureStore.Get", func(ctx context.Context) error {
			_, err := st.WebhookCaptures.Get(ctx, "source:flow-1/hook")
			return err
		}},
	}

	errRollBack := errors.New("roll back")
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()

			var callErr error
			err := st.WithinTx(ctx, func(txCtx context.Context) error {
				callErr = step.call(txCtx)
				return errRollBack
			})
			require.ErrorIs(t, err, errRollBack)
			require.NoError(t, callErr, "%s must join the ambient transaction rather than escape to the pool", step.name)
		})
	}
}
