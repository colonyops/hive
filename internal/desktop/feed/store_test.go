package feed

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStoreAt returns a store rooted in dir: the profiles config and the
// state files live side by side, which is all tests need.
func newStoreAt(dir string) *Store {
	return NewStore(filepath.Join(dir, "profiles.yaml"), dir)
}

func TestStoreCreateProfilePersists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)

	def, err := store.CreateProfile("  Frontend Triage  ")
	require.NoError(t, err)
	assert.Equal(t, "frontend-triage", def.ID)
	assert.Equal(t, "Frontend Triage", def.Name)
	assert.Len(t, def.Feeds, 3)

	// A fresh store instance reads the persisted file: the profile plus the
	// seeded default sources its feeds reference.
	fresh := newStoreAt(dir)
	reloaded, err := fresh.Profiles()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	assert.Equal(t, def.ID, reloaded[0].ID)
	assert.Equal(t, "notifications-inbox", reloaded[0].Feeds[1].ID)
	assert.Equal(t, []string{"inbox"}, reloaded[0].Feeds[1].Sources)

	sources, err := fresh.Sources()
	require.NoError(t, err)
	require.Len(t, sources, 3)
	assert.Equal(t, "my-prs", sources[0].ID)

	assert.FileExists(t, filepath.Join(dir, "profiles.yaml"))
}

func TestStoreCreateProfileUniqueIDs(t *testing.T) {
	t.Parallel()

	store := newStoreAt(t.TempDir())

	first, err := store.CreateProfile("Work")
	require.NoError(t, err)
	second, err := store.CreateProfile("Work")
	require.NoError(t, err)

	assert.Equal(t, "work", first.ID)
	assert.Equal(t, "work-2", second.ID)

	// The second profile reuses the sources the first one seeded.
	sources, err := store.Sources()
	require.NoError(t, err)
	assert.Len(t, sources, 3)
}

func TestStoreCreateProfileRejectsEmptyName(t *testing.T) {
	t.Parallel()

	_, err := newStoreAt(t.TempDir()).CreateProfile("   ")
	require.Error(t, err)
}

func TestStoreCreateProfilePreservesComments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`# my dotfiles-managed feeds
sources:
  # one broad query
  - id: involving-me
    kind: search
    query: "is:open involves:@me"
profiles:
  # the day job
  - id: work
    name: Work
    feeds:
      - id: prs
        name: My PRs
        sources: [involving-me]
`), 0o600))

	store := newStoreAt(dir)
	_, err := store.CreateProfile("Side Projects")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# my dotfiles-managed feeds")
	assert.Contains(t, string(data), "# one broad query")
	assert.Contains(t, string(data), "# the day job")
	assert.Contains(t, string(data), "side-projects")

	// The appended document round-trips through a full load.
	fresh := newStoreAt(dir)
	profiles, err := fresh.Profiles()
	require.NoError(t, err)
	require.Len(t, profiles, 2)
	assert.Equal(t, "work", profiles[0].ID)
	assert.Equal(t, "side-projects", profiles[1].ID)
	assert.Len(t, profiles[1].Feeds, 3)

	// Default sources were appended after the user's own.
	sources, err := fresh.Sources()
	require.NoError(t, err)
	require.Len(t, sources, 4)
	assert.Equal(t, "involving-me", sources[0].ID)
}

func TestStoreCreateProfileRefusesBrokenConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte("profiles: ["), 0o600))

	_, err := newStoreAt(dir).CreateProfile("Work")
	require.ErrorContains(t, err, "profiles.yaml")
}

func TestStoreCreateSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	_, err := store.CreateProfile("Work")
	require.NoError(t, err)

	created, err := store.CreateSource(SourceDef{ID: "Team Reviews!", Kind: "search", Query: "is:open review-requested:@me"})
	require.NoError(t, err)
	assert.Equal(t, "team-reviews", created.ID, "id is slugified")

	// Same requested ID uniquifies with a -2 suffix.
	again, err := store.CreateSource(SourceDef{ID: "team-reviews", Kind: "search", Query: "is:open review-requested:@me org:acme"})
	require.NoError(t, err)
	assert.Equal(t, "team-reviews-2", again.ID)

	sources, err := newStoreAt(dir).Sources()
	require.NoError(t, err)
	assert.Len(t, sources, 5) // 3 defaults + 2 created
}

func TestStoreCreateSourceRejectsInvalidBeforeWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	_, err := store.CreateProfile("Work")
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(dir, "profiles.yaml"))
	require.NoError(t, err)

	_, err = store.CreateSource(SourceDef{ID: "bad", Kind: "search"}) // no query
	require.ErrorContains(t, err, "requires a query")
	_, err = store.CreateSource(SourceDef{ID: "!!!", Kind: "notifications"})
	require.ErrorContains(t, err, "id is empty")
	_, err = store.CreateSource(SourceDef{ID: "big", Kind: "notifications", Limit: 51})
	require.ErrorContains(t, err, "page cap")

	after, err := os.ReadFile(filepath.Join(dir, "profiles.yaml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "rejected creates leave the file untouched")
}

func TestStoreCreateFeed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	profile, err := store.CreateProfile("Work")
	require.NoError(t, err)

	created, err := store.CreateFeed(profile.ID, FeedDef{
		Name:    "Bug Triage",
		Sources: []string{"inbox"},
		Filters: FilterDef{Labels: []string{"bug"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "bug-triage", created.ID, "id derives from the name")

	// Same name uniquifies within the profile.
	again, err := store.CreateFeed(profile.ID, FeedDef{Name: "Bug Triage", Sources: []string{"inbox"}})
	require.NoError(t, err)
	assert.Equal(t, "bug-triage-2", again.ID)

	def, err := newStoreAt(dir).FeedDefFor(profile.ID, "bug-triage")
	require.NoError(t, err)
	assert.Equal(t, "Bug Triage", def.Name)
	assert.Equal(t, []string{"inbox"}, def.Sources)
	assert.Equal(t, []string{"bug"}, def.Filters.Labels)
}

func TestStoreCreateFeedRejectsInvalidBeforeWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	profile, err := store.CreateProfile("Work")
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(dir, "profiles.yaml"))
	require.NoError(t, err)

	_, err = store.CreateFeed("ghost", FeedDef{Name: "X", Sources: []string{"inbox"}})
	require.ErrorContains(t, err, "unknown profile")
	_, err = store.CreateFeed(profile.ID, FeedDef{Name: "X", Sources: []string{"ghost"}})
	require.ErrorContains(t, err, "unknown source")
	_, err = store.CreateFeed(profile.ID, FeedDef{Name: "X"})
	require.ErrorContains(t, err, "at least one source")
	_, err = store.CreateFeed(profile.ID, FeedDef{Name: "  ", Sources: []string{"inbox"}})
	require.ErrorContains(t, err, "name is required")

	after, err := os.ReadFile(filepath.Join(dir, "profiles.yaml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "rejected creates leave the file untouched")
}

func TestStoreUpdateFeed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`# keep me
sources:
  - id: inbox
    kind: notifications
profiles:
  - id: work
    name: Work
    feeds:
      - id: a
        name: A
        sources: [inbox]
`), 0o600))
	store := newStoreAt(dir)

	err := store.UpdateFeed("work", "a", FeedDef{
		ID:      "ignored", // the feed keeps its ID
		Name:    "Mentions",
		Sources: []string{"inbox"},
		Filters: FilterDef{Reasons: []string{"mention"}},
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "# keep me")

	def, err := store.FeedDefFor("work", "a")
	require.NoError(t, err)
	assert.Equal(t, "a", def.ID)
	assert.Equal(t, "Mentions", def.Name)
	assert.Equal(t, []string{"mention"}, def.Filters.Reasons)

	require.ErrorContains(t, store.UpdateFeed("work", "ghost", FeedDef{Name: "X", Sources: []string{"inbox"}}), "not found")
	require.ErrorContains(t, store.UpdateFeed("ghost", "a", FeedDef{Name: "X", Sources: []string{"inbox"}}), "not found")
}

func TestStoreUpdateFeedRejectsInvalidBeforeWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	profile, err := store.CreateProfile("Work")
	require.NoError(t, err)
	before, err := os.ReadFile(filepath.Join(dir, "profiles.yaml"))
	require.NoError(t, err)

	err = store.UpdateFeed(profile.ID, "my-open-prs", FeedDef{
		Name:    "Broken",
		Sources: []string{"my-prs"},
		Filters: FilterDef{Repos: []string{"acme/["}},
	})
	require.ErrorContains(t, err, "invalid repos glob")

	after, err := os.ReadFile(filepath.Join(dir, "profiles.yaml"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "rejected updates leave the file untouched")
}

func TestStoreFeedDefForUnknown(t *testing.T) {
	t.Parallel()

	store := newStoreAt(t.TempDir())
	profile, err := store.CreateProfile("Work")
	require.NoError(t, err)

	_, err = store.FeedDefFor(profile.ID, "ghost")
	require.ErrorContains(t, err, "unknown feed")
	_, err = store.FeedDefFor("ghost", "my-open-prs")
	require.ErrorContains(t, err, "unknown profile")
}

func TestStoreEmptyDirHasNoProfiles(t *testing.T) {
	t.Parallel()

	store := newStoreAt(t.TempDir())
	profiles, err := store.Profiles()
	require.NoError(t, err)
	assert.Empty(t, profiles)

	sources, err := store.Sources()
	require.NoError(t, err)
	assert.Empty(t, sources)
}

func TestStoreReloadKeepsLastGoodOnError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	def, err := store.CreateProfile("Work")
	require.NoError(t, err)

	// Break the file behind the store's back, as a live edit would.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte("profiles: ["), 0o600))
	require.Error(t, store.Reload())

	profiles, err := store.Profiles()
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, def.ID, profiles[0].ID)

	sources, err := store.Sources()
	require.NoError(t, err)
	assert.Len(t, sources, 3, "last-good sources are retained too")

	info := store.ConfigInfo()
	assert.False(t, info.Valid)
	assert.NotEmpty(t, info.Error)
}

func TestStoreReloadPicksUpExternalEdit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	_, err := store.CreateProfile("Work")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "profiles.yaml"), []byte(`sources:
  - id: inbox
    kind: notifications
profiles:
  - id: oss
    name: Open Source
    feeds:
      - id: inbox-feed
        name: Inbox
        sources: [inbox]
        filters:
          repos: ["colonyops/*"]
`), 0o600))
	require.NoError(t, store.Reload())

	profiles, err := store.Profiles()
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "oss", profiles[0].ID)
	assert.Equal(t, []string{"colonyops/*"}, profiles[0].Feeds[0].Filters.Repos)
}

func TestStoreConfigInfoMissingFile(t *testing.T) {
	t.Parallel()

	info := newStoreAt(t.TempDir()).ConfigInfo()
	assert.False(t, info.Exists)
	assert.True(t, info.Valid)
	assert.Contains(t, info.YAML, "sources:")
	assert.Contains(t, info.YAML, "profiles:")
}

func TestStoreMarkReadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := newStoreAt(dir)
	readAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	_, ok := store.ReadAt("o/r#1")
	assert.False(t, ok)

	require.NoError(t, store.MarkRead("o/r#1", readAt))

	got, ok := store.ReadAt("o/r#1")
	require.True(t, ok)
	assert.True(t, got.Equal(readAt))

	// Persisted across instances.
	got, ok = newStoreAt(dir).ReadAt("o/r#1")
	require.True(t, ok)
	assert.True(t, got.Equal(readAt))
}

func TestProfileLetter(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "F", ProfileLetter("frontend triage"))
	assert.Equal(t, "9", ProfileLetter("9lives"))
	assert.Equal(t, "H", ProfileLetter("  hive"))
	assert.Equal(t, "?", ProfileLetter("---"))
}
