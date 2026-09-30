package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/profileimg"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime"
	"github.com/colonyops/hive/cmd/desktop/internal/app/runtime/js"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sourcemark"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/exec"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/webhook"
)

// seedRef is the account a seeded starter graph fetches as; seededCreds is a
// credential store holding it, which is what makes FlowsService.Create seed a
// profile rather than leave it empty.
const seedRef = "github/octocat"

func seededCreds(t *testing.T) credentials.Store {
	t.Helper()
	creds := credentials.NewMemoryStore()
	ref, err := credentials.ParseRef(seedRef)
	require.NoError(t, err)
	require.NoError(t, creds.Set(ref, "token"))
	return creds
}

func testImages(t *testing.T) *profileimg.Store {
	t.Helper()
	return profileimg.NewStore(t.TempDir())
}

func testMarks(t *testing.T) *sourcemark.Store {
	t.Helper()
	return sourcemark.NewStore(t.TempDir())
}

// testScripts is the language registry the service resolves function nodes
// through, wired the same way App wires the real one.
func testScripts() *runtime.ScriptRegistry {
	scripts := runtime.NewScriptRegistry()
	scripts.Register(js.New(runtime.NewScriptPool(0)))
	return scripts
}

func testFlowsService(t *testing.T, d FlowsDeps) *FlowsService {
	t.Helper()
	if d.Events == nil {
		d.Events = newTestBus(t)
	}
	return newFlowsService(d)
}

// pngBytes encodes a small non-square opaque image the normalizer can decode.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 24))
	for y := range 24 {
		for x := range 40 {
			img.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 10), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// markableSourceFlow is a minimal flow carrying one of each source that can
// hold an image mark, wired to a feed, so the node-image methods have real
// nodes to target.
func markableSourceFlow() flow.Flow {
	return flow.Flow{
		ID: "hooks", Name: "Hooks", Enabled: true,
		Nodes: []flow.Node{
			{ID: "hook", Type: "sources.webhook", Config: flow.NewSourceConfig(webhook.Descriptor.Type, &webhook.Config{Path: "ci"})},
			{ID: "run", Type: "sources.exec", Config: flow.NewSourceConfig(exec.Descriptor.Type, &exec.Config{Command: "echo '[]'", Timeout: connector.Duration(30 * time.Second)})},
			{ID: "inbox", Type: "feed", Name: "Inbox", Config: &flow.FeedConfig{}},
		},
		Wires: []flow.Wire{{From: "hook", To: "inbox"}, {From: "run", To: "inbox"}},
	}
}

func TestFlowsServiceSetOrderPersistsAndApplies(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	for _, id := range []string{"hive", "personal", "recipinned"} {
		require.NoError(t, flows.Save(flow.Flow{ID: id, Name: id, Enabled: true}))
	}
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.yaml"))
	bus := newTestBus(t)
	ch := subscribeEvents[events.FlowsUpdated](t, bus)
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts(), Settings: settingsStore, Events: bus})

	require.NoError(t, service.SetOrder(t.Context(), []string{"personal", "hive"}))

	ids := make([]string, 0, 3)
	for _, st := range service.Statuses(t.Context()) {
		ids = append(ids, st.ID)
	}
	assert.Equal(t, []string{"personal", "hive", "recipinned"}, ids, "the live listing reorders without a reload")
	got := requireEvents(t, ch, 1)
	assert.Equal(t, "save", got[0].Reason, "a reorder notifies, so the rail re-reads")

	persisted, err := settingsStore.Persisted()
	require.NoError(t, err)
	assert.Equal(t, []string{"personal", "hive"}, persisted.Profiles.Order)
}

// An id naming no flow is written as given: refusing it would make deleting a
// profile able to fail an unrelated reorder, and the reader already ignores it.
func TestFlowsServiceSetOrderKeepsUnknownIDs(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	require.NoError(t, flows.Save(flow.Flow{ID: "personal", Name: "Personal", Enabled: true}))
	settingsStore := settings.NewStore(filepath.Join(t.TempDir(), "settings.yaml"))
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts(), Settings: settingsStore})

	require.NoError(t, service.SetOrder(t.Context(), []string{"deleted", "personal"}))

	persisted, err := settingsStore.Persisted()
	require.NoError(t, err)
	assert.Equal(t, []string{"deleted", "personal"}, persisted.Profiles.Order)
	assert.Equal(t, "personal", service.Statuses(t.Context())[0].ID)
}

func TestFlowsServiceNodeImageLifecycle(t *testing.T) {
	for _, nodeID := range []string{"hook", "run"} {
		t.Run(nodeID, func(t *testing.T) {
			flows := flow.NewFlowStore(t.TempDir(), nil)
			require.NoError(t, flows.Save(markableSourceFlow()))
			bus := newTestBus(t)
			ch := subscribeEvents[events.FlowsUpdated](t, bus)
			service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts(), Events: bus})

			hash, err := service.SetNodeImage(t.Context(), "hooks", nodeID, pngBytes(t))
			require.NoError(t, err)
			assert.True(t, sourcemark.ValidHash(hash))
			requireEvents(t, ch, 1)

			data, err := service.NodeImage(t.Context(), "hooks", nodeID)
			require.NoError(t, err)
			assert.NotEmpty(t, data)

			require.NoError(t, service.ClearNodeImage(t.Context(), "hooks", nodeID))
			data, err = service.NodeImage(t.Context(), "hooks", nodeID)
			require.NoError(t, err)
			assert.Nil(t, data, "a cleared node reads as no image")
		})
	}
}

func TestFlowsServiceNodeImageErrorKinds(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	require.NoError(t, flows.Save(markableSourceFlow()))
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	_, err := service.SetNodeImage(t.Context(), "nope", "hook", pngBytes(t))
	assert.Equal(t, KindNotFound, KindOf(err), "unknown flow is not-found")

	_, err = service.SetNodeImage(t.Context(), "hooks", "ghost", pngBytes(t))
	assert.Equal(t, KindNotFound, KindOf(err), "unknown node is not-found")

	_, err = service.SetNodeImage(t.Context(), "hooks", "inbox", pngBytes(t))
	assert.Equal(t, KindInvalid, KindOf(err), "a node with no image mark cannot carry one")

	_, err = service.SetNodeImage(t.Context(), "hooks", "hook", []byte("not an image"))
	assert.Equal(t, KindInvalid, KindOf(err), "an undecodable image is rejected")
}

// The editor's two-step path: the bytes are stored before the graph save that
// records the hash, so the store must round-trip without a flow.
func TestFlowsServiceMarkImageRoundTrip(t *testing.T) {
	service := testFlowsService(t, FlowsDeps{Flows: flow.NewFlowStore(t.TempDir(), nil), Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	hash, err := service.StoreMarkImage(t.Context(), pngBytes(t))
	require.NoError(t, err)
	require.True(t, sourcemark.ValidHash(hash))

	data, ok, err := service.MarkImage(t.Context(), hash)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.NotEmpty(t, data)

	// A hash with no stored file resolves as absent, not an error — the feed
	// falls back to the glyph.
	_, ok, err = service.MarkImage(t.Context(), "0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFlowsServiceStoreMarkImageRejectsBadInput(t *testing.T) {
	service := testFlowsService(t, FlowsDeps{Flows: flow.NewFlowStore(t.TempDir(), nil), Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	_, err := service.StoreMarkImage(t.Context(), []byte("not an image"))
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestFlowsServiceDeleteFlowPurgesPipelineStateAndRetriesMissingFiles(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	flows := flow.NewFlowStore(t.TempDir(), nil)
	st := stores.New(db, stores.Options{})
	service := testFlowsService(t, FlowsDeps{Flows: flows, Stores: st, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})
	created, err := service.Create(t.Context(), "Profile")
	require.NoError(t, err)
	_, err = stores.NewSeed(db).InboxItem(t.Context(), stores.InboxItem{
		ProfileID: created.ID, SourceKind: "github", ExternalID: "item", Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)
	_, err = st.EventLog.Append(t.Context(), "source:"+created.ID+"/source", "item", []byte(`{}`))
	require.NoError(t, err)

	require.NoError(t, service.Delete(t.Context(), created.ID))
	for _, table := range []string{"inbox_item", "event_log", "consumer_offset", "source_head"} {
		var count int
		require.NoError(t, db.Conn().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count))
		assert.Zero(t, count, table)
	}

	assert.Equal(t, KindNotFound, KindOf(service.Delete(t.Context(), created.ID)),
		"with the files and the rows both gone there is nothing left to delete")
}

// The files-first retry path: a delete whose purge failed leaves the flow file
// gone and the rows behind, and the id is the only handle left on them — so the
// retry has to be accepted even though nothing on disk backs it any more.
func TestFlowsServiceDeleteRetriesAPurgeThatLeftRowsBehind(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	flows := flow.NewFlowStore(t.TempDir(), nil)
	st := stores.New(db, stores.Options{})
	service := testFlowsService(t, FlowsDeps{Flows: flows, Stores: st, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})
	created, err := service.Create(t.Context(), "Profile")
	require.NoError(t, err)
	_, err = stores.NewSeed(db).InboxItem(t.Context(), stores.InboxItem{
		ProfileID: created.ID, SourceKind: "github", ExternalID: "item", Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)

	// Stand in for the interrupted delete: the files went, the purge did not.
	require.NoError(t, flows.Delete(created.ID))

	require.NoError(t, service.Delete(t.Context(), created.ID))
	var count int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), "SELECT COUNT(*) FROM inbox_item").Scan(&count))
	assert.Zero(t, count)
}

type purgeProfileFixture struct {
	itemID int64
	topic  string
}

func seedPurgeProfileRows(t *testing.T, db *queries.DB, profileID string) purgeProfileFixture {
	t.Helper()
	ctx := t.Context()
	topic := "source:" + profileID + "/src"
	st := stores.New(db, stores.Options{})
	seed := stores.NewSeed(db)

	item, err := seed.InboxItem(ctx, stores.InboxItem{
		ProfileID: profileID, SourceKind: "github", SourceScope: "s", ExternalID: profileID + "-item",
		Payload: []byte(`{}`), Lifecycle: "active",
	})
	require.NoError(t, err)
	_, err = seed.InboxEvent(ctx, stores.InboxEvent{
		ItemID: item.ID, Kind: "observed", Transition: "none", Attention: "trivial", Detail: []byte(`{}`), CreatedAt: 1,
	})
	require.NoError(t, err)
	require.NoError(t, st.FeedClaims.Upsert(ctx, models.FeedClaim{
		ProfileID: profileID, FeedID: profileID + "/feed", ItemID: item.ID, SourceID: topic,
	}))
	require.NoError(t, st.ItemSessions.Link(ctx, profileID+"-sess", models.ItemRef{
		ProfileID: profileID, SourceKind: "github", SourceScope: "s", ExternalID: profileID + "-item",
	}))
	_, err = st.EventLog.Append(ctx, topic, profileID+"-item", []byte(`{}`))
	require.NoError(t, err)
	require.NoError(t, seed.ConsumerOffset(ctx, profileID, 1))
	require.NoError(t, st.SourceHeads.Upsert(ctx, topic, profileID+"-item", []byte(`{}`)))
	require.NoError(t, st.NodeKV.Set(ctx, profileID, "node-a", "k", "v", 0))
	return purgeProfileFixture{itemID: item.ID, topic: topic}
}

func assertPurgeProfileRowCounts(t *testing.T, db *queries.DB, profileID string, fx purgeProfileFixture, want int) {
	t.Helper()
	ctx := t.Context()
	assertCount := func(table, query string, args ...any) {
		t.Helper()
		var n int
		require.NoError(t, db.Conn().QueryRowContext(ctx, query, args...).Scan(&n))
		assert.Equal(t, want, n, "%s for profile %q", table, profileID)
	}
	assertCount("inbox_item", `SELECT COUNT(*) FROM inbox_item WHERE profile_id = ?`, profileID)
	assertCount("inbox_event", `SELECT COUNT(*) FROM inbox_event WHERE item_id = ?`, fx.itemID)
	assertCount("feed_membership_claim", `SELECT COUNT(*) FROM feed_membership_claim WHERE profile_id = ?`, profileID)
	assertCount("item_session", `SELECT COUNT(*) FROM item_session WHERE profile_id = ?`, profileID)
	assertCount("event_log", `SELECT COUNT(*) FROM event_log WHERE topic = ?`, fx.topic)
	assertCount("consumer_offset", `SELECT COUNT(*) FROM consumer_offset WHERE consumer = ?`, profileID)
	assertCount("source_head", `SELECT COUNT(*) FROM source_head WHERE topic = ?`, fx.topic)
	assertCount("node_kv", `SELECT COUNT(*) FROM node_kv WHERE flow_id = ?`, profileID)
}

func TestFlowsServicePurgeProfile_DeletesEveryOwnedRowAndLeavesOtherProfilesIntact(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	st := stores.New(db, stores.Options{})
	service := testFlowsService(t, FlowsDeps{Flows: flow.NewFlowStore(t.TempDir(), nil), Stores: st, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	target := seedPurgeProfileRows(t, db, "p")
	other := seedPurgeProfileRows(t, db, "other")

	require.NoError(t, service.purgeProfile(t.Context(), "p"))
	require.NoError(t, service.purgeProfile(t.Context(), "p"), "a second purge of the same profile is a no-op")

	assertPurgeProfileRowCounts(t, db, "p", target, 0)
	assertPurgeProfileRowCounts(t, db, "other", other, 1)
}

// A mid-purge failure must leave every table the earlier deletes already
// touched exactly as it was: node_kv is purgeProfile's last delete, so
// dropping it forces a real error after the other six ran inside the same
// transaction, and only a rollback keeps this profile's inbox_item row from
// being half-purged.
func TestFlowsServicePurgeProfile_RollsBackTheWholeTransactionOnFailure(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	st := stores.New(db, stores.Options{})
	service := testFlowsService(t, FlowsDeps{Flows: flow.NewFlowStore(t.TempDir(), nil), Stores: st, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	target := seedPurgeProfileRows(t, db, "p")

	_, err = db.Conn().ExecContext(t.Context(), `DROP TABLE node_kv`)
	require.NoError(t, err)

	require.Error(t, service.purgeProfile(t.Context(), "p"))

	var count int
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM inbox_item WHERE id = ?`, target.itemID).Scan(&count))
	assert.Equal(t, 1, count, "a failed purge must roll back every delete already made in the same transaction")
	require.NoError(t, db.Conn().QueryRowContext(t.Context(), `SELECT COUNT(*) FROM event_log WHERE topic = ?`, target.topic).Scan(&count))
	assert.Equal(t, 1, count, "the event log delete that ran before the failure must also roll back")
}

// A profile whose flow file does not parse still exists — deleting it is how
// that gets resolved, so the delete must not be gated on the file loading.
func TestFlowsServiceDeleteRemovesAProfileThatDoesNotParse(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("version: 1\nnodes: [\n"), 0o600))
	flows := flow.NewFlowStore(dir, nil)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	st := stores.New(db, stores.Options{})
	service := testFlowsService(t, FlowsDeps{Flows: flows, Stores: st, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	require.NoError(t, service.Delete(t.Context(), "broken"), "the file is gone and there are no rows to purge")
	assert.NoFileExists(t, filepath.Join(dir, "broken.yaml"))
}

func TestFlowsServiceDeleteReportsAnUnknownProfileAsNotFound(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	st := stores.New(db, stores.Options{})
	service := testFlowsService(t, FlowsDeps{Flows: flows, Stores: st, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	assert.Equal(t, KindNotFound, KindOf(service.Delete(t.Context(), "never-existed")))
}

// A profile always exists: first run never asks for one, and the rail never
// renders empty. A broken file is a profile too - it is listed with its error,
// and a Default written beside it would hide that.
func TestFlowsServiceEnsureProfileMakesADefaultOnlyWhenThereAreNone(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		flows := flow.NewFlowStore(t.TempDir(), nil)
		service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: credentials.NewMemoryStore(), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

		require.NoError(t, service.EnsureProfile(t.Context()))

		statuses := flows.Statuses()
		require.Len(t, statuses, 1)
		assert.Equal(t, "default", statuses[0].ID)
		assert.Equal(t, DefaultProfileName, statuses[0].Flow.Name)
		assert.Empty(t, statuses[0].Flow.Nodes, "with no account connected there is nothing to seed")
		require.NoError(t, service.EnsureProfile(t.Context()))
		assert.Len(t, flows.Statuses(), 1, "a second call is a no-op")
	})

	t.Run("only a broken file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("version: 1\nnodes: [\n"), 0o600))
		flows := flow.NewFlowStore(dir, nil)
		service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: credentials.NewMemoryStore(), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

		require.NoError(t, service.EnsureProfile(t.Context()))

		assert.NoFileExists(t, filepath.Join(dir, "default.yaml"))
	})
}

func TestFlowsServiceDeleteOfTheLastProfileLeavesADefaultBehind(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	st := stores.New(db, stores.Options{})
	service := testFlowsService(t, FlowsDeps{Flows: flows, Stores: st, Creds: credentials.NewMemoryStore(), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})
	created, err := service.Create(t.Context(), "Only one")
	require.NoError(t, err)

	require.NoError(t, service.Delete(t.Context(), created.ID))

	statuses := flows.Statuses()
	require.Len(t, statuses, 1)
	assert.Equal(t, DefaultProfileName, statuses[0].Flow.Name)
	assert.NotEqual(t, created.ID, statuses[0].ID)
}

func TestFlowsServiceCreateSeedsWithTheOneConnectedAccount(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	created, err := service.Create(t.Context(), "Triage")
	require.NoError(t, err)
	assert.NotEmpty(t, created.Nodes, "one connected account is enough to seed the starter graph")
	assert.NotEmpty(t, flows.GetLayout(created.ID).Nodes, "the seeded nodes need canvas positions")
}

// A profile is the thing that exists without any credential: first run
// creates it before it offers to connect anything, so an unseeded create is
// the expected path there rather than a failure.
func TestFlowsServiceCreateWithoutAnUnambiguousAccountMakesAnEmptyWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name string
		refs []string
	}{
		{"no account connected", nil},
		{"several accounts connected", []string{"github/octocat", "github/hubot"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			creds := credentials.NewMemoryStore()
			for _, raw := range tc.refs {
				ref, err := credentials.ParseRef(raw)
				require.NoError(t, err)
				require.NoError(t, creds.Set(ref, "token"))
			}
			service := testFlowsService(t, FlowsDeps{Flows: flow.NewFlowStore(t.TempDir(), nil), Creds: creds, Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

			created, err := service.Create(t.Context(), "Triage")
			require.NoError(t, err)
			assert.Empty(t, created.Nodes)
		})
	}
}

func TestFlowsServiceSeedStarterFillsAnEmptyWorkspace(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	creds := credentials.NewMemoryStore()
	bus := newTestBus(t)
	ch := subscribeEvents[events.FlowsUpdated](t, bus)
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: creds, Images: testImages(t), Marks: testMarks(t), Scripts: testScripts(), Events: bus})

	// The first-run order: the profile exists before the account does.
	created, err := service.Create(t.Context(), "Triage")
	require.NoError(t, err)
	require.Empty(t, created.Nodes)

	_, err = service.SeedStarter(t.Context(), created.ID)
	require.ErrorContains(t, err, "Connect exactly one GitHub account")

	ref, err := credentials.ParseRef(seedRef)
	require.NoError(t, err)
	require.NoError(t, creds.Set(ref, "token"))

	seeded, err := service.SeedStarter(t.Context(), created.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, seeded.Nodes)
	assert.NotEmpty(t, flows.GetLayout(created.ID).Nodes)
	assert.Equal(t, created.Name, seeded.Name, "seeding keeps the name the user chose")
	requireEvents(t, ch, 2)

	// Appending a second starter graph onto a graph someone has since edited
	// is not a mistake they can undo, so a populated profile is refused.
	_, err = service.SeedStarter(t.Context(), created.ID)
	require.ErrorContains(t, err, "already has nodes")

	_, err = service.SeedStarter(t.Context(), "missing")
	require.ErrorContains(t, err, "not found")
}

func TestFlowsServiceSetFlowEnabled(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	created, err := flows.Create("Triage", starterSeed(seedRef))
	require.NoError(t, err)

	bus := newTestBus(t)
	ch := subscribeEvents[events.FlowsUpdated](t, bus)
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts(), Events: bus})
	summary, err := service.SetEnabled(t.Context(), created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, created.ID, summary.ID)
	assert.False(t, summary.Enabled)
	requireEvents(t, ch, 1)

	stored, ok := flows.Get(created.ID)
	require.True(t, ok)
	assert.False(t, stored.Enabled)
}

func TestFlowsServiceSetFlowEnabledDoesNotEmitOnFailure(t *testing.T) {
	bus := newTestBus(t)
	ch := subscribeEvents[events.FlowsUpdated](t, bus)
	service := testFlowsService(t, FlowsDeps{Flows: flow.NewFlowStore(t.TempDir(), nil), Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts(), Events: bus})

	_, err := service.SetEnabled(t.Context(), "missing", false)
	require.Error(t, err)
	requireNoMoreEvents(t, ch)
}

func TestFlowsServiceProfileImageLifecycle(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	created, err := flows.Create("Triage", starterSeed(seedRef))
	require.NoError(t, err)

	bus := newTestBus(t)
	ch := subscribeEvents[events.FlowsUpdated](t, bus)
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts(), Events: bus})

	set, err := service.SetProfileImage(t.Context(), created.ID, pngBytes(t))
	require.NoError(t, err)
	require.NotEmpty(t, set.Image, "the flow records a content-hash reference")
	requireEvents(t, ch, 1)

	data, err := service.ProfileImage(t.Context(), created.ID)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 128, 128), img.Bounds(), "stored avatar is a 128px square PNG")

	// A graph save carries no image field; the store must preserve the avatar
	// rather than drop it.
	editable, ok := flows.Get(created.ID)
	require.True(t, ok)
	editable.Image = ""
	require.NoError(t, service.Save(t.Context(), editable))
	preserved, err := service.ProfileImage(t.Context(), created.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, preserved, "a graph save preserves the image it does not carry")
	stored, ok := flows.Get(created.ID)
	require.True(t, ok)
	assert.Equal(t, set.Image, stored.Image, "the reference survives a graph save")

	cleared, err := service.ClearProfileImage(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Empty(t, cleared.Image)
	gone, err := service.ProfileImage(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Empty(t, gone, "clearing removes the stored bytes")
}

func TestFlowsServiceSetProfileImageRejectsBadInput(t *testing.T) {
	flows := flow.NewFlowStore(t.TempDir(), nil)
	created, err := flows.Create("Triage", starterSeed(seedRef))
	require.NoError(t, err)
	service := testFlowsService(t, FlowsDeps{Flows: flows, Creds: seededCreds(t), Images: testImages(t), Marks: testMarks(t), Scripts: testScripts()})

	_, err = service.SetProfileImage(t.Context(), created.ID, []byte("not an image"))
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
	stored, ok := flows.Get(created.ID)
	require.True(t, ok)
	assert.Empty(t, stored.Image, "a rejected image leaves the flow unreferenced")

	_, err = service.SetProfileImage(t.Context(), "missing", pngBytes(t))
	require.Error(t, err)
	assert.Equal(t, KindNotFound, KindOf(err))
}
