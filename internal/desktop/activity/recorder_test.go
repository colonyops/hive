package activity

import (
	"context"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T, opts Options) *Store {
	t.Helper()
	db, err := pipelinedb.Open(t.TempDir(), pipelinedb.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db, opts)
}

func TestStoreAppendRoundTrip(t *testing.T) {
	emitted := 0
	var lastEmittedID int64
	store := newTestStore(t, Options{Emit: func(id int64) { emitted++; lastEmittedID = id }})
	ctx := context.Background()

	stored, err := store.Append(ctx, ActionRun("Reproduce & fix", "exit 0"))
	require.NoError(t, err)
	require.NotZero(t, stored.ID)
	require.NotZero(t, stored.CreatedAt)
	require.Equal(t, CategoryAction, stored.Category)
	require.Equal(t, SeveritySuccess, stored.Severity)
	require.Equal(t, "Ran Reproduce & fix", stored.Title)
	require.Equal(t, 1, emitted, "emit fires once per successful append")
	require.Equal(t, stored.ID, lastEmittedID, "emit carries the new event id")

	events, err := store.List(ctx, 0, 50)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, stored, events[0])
}

func TestStoreListNewestFirstAndCursor(t *testing.T) {
	// A fixed, monotonically advancing clock keeps created_at deterministic;
	// ordering itself is by the autoincrement id, not the timestamp.
	now := time.Unix(0, 0)
	store := newTestStore(t, Options{Now: func() time.Time { now = now.Add(time.Second); return now }})
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := store.Append(ctx, ActionRun("Reproduce & fix", "exit 0"))
		require.NoError(t, err)
	}

	page1, err := store.List(ctx, 0, 3)
	require.NoError(t, err)
	require.Len(t, page1, 3)
	require.Greater(t, page1[0].ID, page1[1].ID, "newest first")
	require.Greater(t, page1[1].ID, page1[2].ID)

	page2, err := store.List(ctx, page1[2].ID, 3)
	require.NoError(t, err)
	require.Len(t, page2, 2, "cursor pages the remainder")
	require.Less(t, page2[0].ID, page1[2].ID)
}

func TestStoreAppendRejectsBadInput(t *testing.T) {
	store := newTestStore(t, Options{})
	ctx := context.Background()

	_, err := store.Append(ctx, Event{Category: CategorySystem, Severity: SeverityInfo})
	require.Error(t, err, "missing title is rejected")

	_, err = store.Append(ctx, Event{Title: "bad", Category: Category("nope"), Severity: SeverityInfo})
	require.Error(t, err, "invalid category is rejected")
}

func TestStoreAppendDefaultsCategoryAndSeverity(t *testing.T) {
	store := newTestStore(t, Options{})
	stored, err := store.Append(context.Background(), Event{Title: "something happened"})
	require.NoError(t, err)
	require.Equal(t, CategorySystem, stored.Category)
	require.Equal(t, SeverityInfo, stored.Severity)
}

func TestConstructors(t *testing.T) {
	cases := []struct {
		name     string
		event    Event
		category Category
		severity Severity
	}{
		{"refresh-failed", RefreshFailed("s", "boom"), CategoryRefresh, SeverityError},
		{"session", SessionCreated("review-pr-2841", "sonnet", "hive/core"), CategorySession, SeveritySuccess},
		{"auto-action", AutoAction("Triage", "triage.default", "#4820"), CategoryAutoAction, SeverityAuto},
		{"action", ActionRun("Reproduce", "exit 0"), CategoryAction, SeveritySuccess},
		{"action-failed", ActionFailed("Reproduce", "exit 1"), CategoryAction, SeverityError},
		{"config", ConfigReloaded("actions.yml", 6), CategoryConfig, SeverityInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.category, tc.event.Category)
			require.Equal(t, tc.severity, tc.event.Severity)
			require.NotEmpty(t, tc.event.Title)
		})
	}
}

func TestRecordInputEvent(t *testing.T) {
	ev, err := RecordInput{Title: "Profile deleted", Severity: "success"}.Event()
	require.NoError(t, err)
	require.Equal(t, CategorySystem, ev.Category, "empty category defaults to system")
	require.Equal(t, SeveritySuccess, ev.Severity)

	_, err = RecordInput{Severity: "success"}.Event()
	require.Error(t, err, "missing title is rejected")

	_, err = RecordInput{Title: "x", Category: "bogus"}.Event()
	require.Error(t, err, "invalid category is rejected")
}
