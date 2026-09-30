package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

type fixedTicker time.Time

func (f fixedTicker) LastTick() time.Time { return time.Time(f) }

type menuBarFixture struct {
	service  *MenuBarService
	flowsDir string
	db       *queries.DB
	bus      *events.Bus
}

func newMenuBarFixture(t *testing.T, polls LastTicker) menuBarFixture {
	t.Helper()
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(`version: 1
name: Work
nodes:
  - { id: src, type: sources.github, credential: github/octocat, kind: search, query: "is:open" }
  - { id: prs, type: feed, name: Reviews }
  - { id: other, type: feed }
wires:
  - { from: src, to: prs }
  - { from: src, to: other }
`), 0o644))

	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	bus := events.New(zerolog.Nop())
	service := newMenuBarService(MenuBarDeps{
		Settings: settings.NewStore(settings.SettingsPath()),
		Flows:    flow.NewFlowStore(dir, nil),
		Items:    stores.New(db, stores.Options{}).InboxItems,
		Catalog:  configuredActionStore(t),
		Polls:    polls,
		Events:   bus,
	})
	return menuBarFixture{service: service, flowsDir: dir, db: db, bus: bus}
}

func (f menuBarFixture) claim(t *testing.T, feedID string, itemID int64) {
	t.Helper()
	require.NoError(t, f.db.UpsertFeedMembershipClaim(t.Context(), queries.UpsertFeedMembershipClaimParams{
		ProfileID: "p", FeedID: feedID, ItemID: itemID, SourceID: "src",
	}))
}

func TestMenuBarSnapshotListsPinnedFeedUpToItsLimit(t *testing.T) {
	polled := time.Date(2026, 9, 24, 15, 4, 0, 0, time.UTC)
	f := newMenuBarFixture(t, fixedTicker(polled))
	pr := insertActionItemSource(t, f.db, "github", "pr-1", "PR", "Add retry budget", map[string]any{"repo": "acme/api", "num": 412, "reason": "review_requested"})
	f.claim(t, "p/prs", pr)
	issue := insertActionItemSource(t, f.db, "github", "issue-1", "Issue", "Rate limiter drops", nil)
	f.claim(t, "p/prs", issue)
	f.claim(t, "p/other", insertActionItem(t, f.db, "issue-2", "Issue", "Elsewhere"))

	require.NoError(t, f.service.SetPins(t.Context(), []MenuBarPin{{Feed: "p/prs", Limit: 1}}))

	snapshot, err := f.service.Snapshot(t.Context())
	require.NoError(t, err)

	require.Len(t, snapshot.Pinned, 1)
	feed := snapshot.Pinned[0]
	assert.Equal(t, "Reviews", feed.Name)
	assert.Equal(t, "p", feed.ProfileID)
	assert.EqualValues(t, 2, feed.Total, "the total counts past the item limit so the menu can offer the rest")
	require.Len(t, feed.Items, 1)
	assert.Equal(t, polled, snapshot.LastPolled)
}

func TestMenuBarItemCarriesApplicableActions(t *testing.T) {
	f := newMenuBarFixture(t, nil)
	pr := insertActionItemSource(t, f.db, "github", "pr-1", "PR", "Add retry budget", map[string]any{"repo": "acme/api", "num": 412, "reason": "review_requested"})
	row, err := stores.New(f.db, stores.Options{}).InboxItems.GetByID(t.Context(), pr)
	require.NoError(t, err)

	item := menuBarItem(row, f.service.runnableActions())

	assert.Equal(t, []MenuBarAction{
		{ID: "review-pr", Label: "Review PR"},
		{ID: "deploy-repo", Label: "Deploy"},
		{ID: "triage-any", Label: "Triage"},
		{ID: "copy-checkout", Label: "Copy checkout command", Clipboard: true},
	}, item.Actions)
}

func TestMenuBarSnapshotMarksUnreadItems(t *testing.T) {
	f := newMenuBarFixture(t, nil)
	row, err := stores.NewSeed(f.db).InboxItem(t.Context(), stores.InboxItem{ProfileID: "p", SourceKind: "github", ExternalID: "x", Title: "Unread", Payload: []byte(`{}`), Unread: true, Lifecycle: "active"})
	require.NoError(t, err)
	f.claim(t, "p/other", row.ID)
	require.NoError(t, f.service.SetPins(t.Context(), []MenuBarPin{{Feed: "p/other"}}))

	snapshot, err := f.service.Snapshot(t.Context())
	require.NoError(t, err)

	require.Len(t, snapshot.Pinned, 1)
	assert.True(t, snapshot.Pinned[0].Items[0].Unread)
	assert.True(t, snapshot.LastPolled.IsZero(), "no producer means no poll time")
}

func TestMenuBarSnapshotSkipsPinsNamingNoFeed(t *testing.T) {
	f := newMenuBarFixture(t, nil)
	require.NoError(t, f.service.SetPins(t.Context(), []MenuBarPin{{Feed: "gone/feed"}, {Feed: "p/missing"}, {Feed: "p/prs"}}))

	snapshot, err := f.service.Snapshot(t.Context())
	require.NoError(t, err)

	require.Len(t, snapshot.Pinned, 1)
	assert.Equal(t, "p/prs", snapshot.Pinned[0].Feed)
}

func TestMenuBarPinsRoundTripAndPublish(t *testing.T) {
	f := newMenuBarFixture(t, nil)
	published := make(chan struct{}, 1)
	cancel := events.Subscribe(t.Context(), f.bus, "test", events.Coalesce(), func(_ context.Context, _ events.MenuBarUpdated) {
		published <- struct{}{}
	})
	t.Cleanup(cancel)

	require.NoError(t, f.service.SetPins(t.Context(), []MenuBarPin{{Feed: "p/prs", Limit: 8}, {Feed: "p/other", Limit: settings.DefaultMenuBarItemLimit}}))

	pins, err := f.service.Pins(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []MenuBarPin{{Feed: "p/prs", Limit: 8}, {Feed: "p/other", Limit: settings.DefaultMenuBarItemLimit}}, pins)

	persisted, err := settings.NewStore(settings.SettingsPath()).Persisted()
	require.NoError(t, err)
	assert.Equal(t, 0, persisted.MenuBar.Feeds[1].Limit, "the default limit is not written out")

	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("SetPins did not publish MenuBarUpdated")
	}
}

func TestMenuBarSetPinsRejectsInvalid(t *testing.T) {
	f := newMenuBarFixture(t, nil)
	cases := map[string][]MenuBarPin{
		"too many":  {{Feed: "p/a"}, {Feed: "p/b"}, {Feed: "p/c"}, {Feed: "p/d"}},
		"duplicate": {{Feed: "p/a"}, {Feed: "p/a"}},
		"malformed": {{Feed: "nofeed"}},
		"limit":     {{Feed: "p/a", Limit: settings.MaxMenuBarItemLimit + 1}},
	}
	for name, pins := range cases {
		t.Run(name, func(t *testing.T) {
			err := f.service.SetPins(t.Context(), pins)
			require.Error(t, err)
			assert.Equal(t, KindInvalid, KindOf(err))
		})
	}
}

func TestMenuBarFeedChoicesListEveryDeclaredFeed(t *testing.T) {
	f := newMenuBarFixture(t, nil)
	assert.Equal(t, []MenuBarFeedChoice{
		{Feed: "p/prs", ProfileName: "Work", Name: "Reviews"},
		{Feed: "p/other", ProfileName: "Work", Name: "other"},
	}, f.service.FeedChoices(t.Context()))
}

func TestMenuBarFeedNamesCarryTheirSidebarFolder(t *testing.T) {
	f := newMenuBarFixture(t, nil)
	require.NoError(t, os.WriteFile(filepath.Join(f.flowsDir, "p.sidebar.yaml"), []byte(`items:
  - folder: { id: f1, name: Code review, feeds: [prs] }
  - feed: other
`), 0o644))
	require.NoError(t, f.service.SetPins(t.Context(), []MenuBarPin{{Feed: "p/prs"}}))

	snapshot, err := f.service.Snapshot(t.Context())
	require.NoError(t, err)

	require.Len(t, snapshot.Pinned, 1)
	assert.Equal(t, MenuBarFeedName{ProfileName: "Work", Folder: "Code review", Name: "Reviews"}, snapshot.Pinned[0].MenuBarFeedName)
	assert.Equal(t, "Code review", f.service.FeedChoices(t.Context())[0].Folder)
}
