package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/activity"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
)

func newTestActivityService(t *testing.T) (*ActivityService, <-chan events.ActivityAppended) {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	bus := newTestBus(t)
	ch := subscribeEvents[events.ActivityAppended](t, bus)
	return newActivityService(stores.New(db, stores.Options{}).ActivityEvents, bus, zerolog.Nop()), ch
}

func TestActivityService_AppendRoundTrip(t *testing.T) {
	service, ch := newTestActivityService(t)
	ctx := t.Context()

	stored, err := service.Append(ctx, activity.ActionRun("Reproduce & fix", "exit 0"))
	require.NoError(t, err)
	require.NotZero(t, stored.ID)
	require.NotZero(t, stored.CreatedAt)
	require.Equal(t, activity.CategoryAction, stored.Category)
	require.Equal(t, activity.SeveritySuccess, stored.Severity)
	require.Equal(t, "Ran Reproduce & fix", stored.Title)
	got := requireEvents(t, ch, 1)
	require.Equal(t, stored.ID, got[0].ID, "events.ActivityAppended carries the new event id")

	listed, err := service.List(ctx, 0, 50)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, stored, listed[0])
}

func TestActivityService_ListNewestFirstAndCursor(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	// A fixed, monotonically advancing clock keeps created_at deterministic;
	// ordering itself is by the autoincrement id, not the timestamp.
	now := time.Unix(0, 0)
	st := stores.New(db, stores.Options{Now: func() time.Time { now = now.Add(time.Second); return now }})
	service := newActivityService(st.ActivityEvents, newTestBus(t), zerolog.Nop())
	ctx := t.Context()

	for range 5 {
		_, err := service.Append(ctx, activity.ActionRun("Reproduce & fix", "exit 0"))
		require.NoError(t, err)
	}

	page1, err := service.List(ctx, 0, 3)
	require.NoError(t, err)
	require.Len(t, page1, 3)
	require.Greater(t, page1[0].ID, page1[1].ID, "newest first")
	require.Greater(t, page1[1].ID, page1[2].ID)

	page2, err := service.List(ctx, page1[2].ID, 3)
	require.NoError(t, err)
	require.Len(t, page2, 2, "cursor pages the remainder")
	require.Less(t, page2[0].ID, page1[2].ID)
}

func TestActivityService_AppendRejectsBadInput(t *testing.T) {
	service, ch := newTestActivityService(t)
	ctx := t.Context()

	_, err := service.Append(ctx, activity.Event{Category: activity.CategorySystem, Severity: activity.SeverityInfo})
	require.Error(t, err, "missing title is rejected")
	require.Equal(t, KindInvalid, KindOf(err))

	_, err = service.Append(ctx, activity.Event{Title: "bad", Category: activity.Category("nope"), Severity: activity.SeverityInfo})
	require.Error(t, err, "invalid category is rejected")
	require.Equal(t, KindInvalid, KindOf(err))
	requireNoMoreEvents(t, ch)
}

func TestActivityService_AppendDefaultsCategoryAndSeverity(t *testing.T) {
	service, _ := newTestActivityService(t)
	stored, err := service.Append(t.Context(), activity.Event{Title: "something happened"})
	require.NoError(t, err)
	require.Equal(t, activity.CategorySystem, stored.Category)
	require.Equal(t, activity.SeverityInfo, stored.Severity)
}

// Record is fire-and-forget, so persistence failures must not reach callers.
func TestActivityService_RecordSwallowsErrors(t *testing.T) {
	service, _ := newTestActivityService(t)
	require.NotPanics(t, func() {
		service.Record(t.Context(), activity.Event{}) // missing title would error from Append
	})
}

// The service's Recorder is the metered one, so a Record through the port both
// lands in the store and counts.
func TestActivityService_RecordCountsTheEvent(t *testing.T) {
	service, _ := newTestActivityService(t)
	attrs := []attribute.KeyValue{
		attribute.String("category", activity.CategoryConfig.String()),
		attribute.String("severity", activity.SeverityInfo.String()),
	}
	before := counterValue(t, "activity.events", attrs...)

	service.Record(t.Context(), activity.ConfigReloaded("actions.yml", 3))

	require.GreaterOrEqual(t, counterValue(t, "activity.events", attrs...)-before, int64(1))
	listed, err := service.List(t.Context(), 0, 10)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "Reloaded actions.yml", listed[0].Title)
}
