package applets

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimal = `
version: 1
title: Thing
`

func TestBuiltinsParse(t *testing.T) {
	entries := loadBuiltins(t.TempDir())
	require.Contains(t, entries, BuilderID)
	body, err := ReadBuiltin(BuilderID, entries[BuilderID].Applet.Entry)
	require.NoError(t, err)
	assert.NotEmpty(t, body)
}

func TestParseDefaults(t *testing.T) {
	applet, err := Parse("thing", []byte(minimal))
	require.NoError(t, err)
	assert.Equal(t, "thing", applet.ID)
	assert.Equal(t, DefaultEntry, applet.Entry)
}

func TestParseRefuses(t *testing.T) {
	for name, body := range map[string]string{
		"wrong version": "version: 2\ntitle: T\n",
		"no title":      "version: 1\n",
		"unknown key":   "version: 1\ntitle: T\nview: table\n",
		"unknown icon":  "version: 1\ntitle: T\nicon: nope\n",
		"entry outside": "version: 1\ntitle: T\nentry: ../outside.html\n",
		"command id":    "version: 1\ntitle: T\ncommands:\n  - {id: not.a.slug, title: C}\n",
		"command twice": "version: 1\ntitle: T\ncommands:\n  - {id: c, title: C}\n  - {id: c, title: D}\n",
		"command title": "version: 1\ntitle: T\ncommands:\n  - {id: c}\n",
		"command icon":  "version: 1\ntitle: T\ncommands:\n  - {id: c, title: C, icon: nope}\n",
		"negative size": "version: 1\ntitle: T\nwidth: -1\n",
		"empty file":    "",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("thing", []byte(body))
			require.Error(t, err)
		})
	}
	_, err := Parse("Bad Name", []byte(minimal))
	require.Error(t, err)
}

func TestWatchedFilesReloadPagesButNotTheirData(t *testing.T) {
	assert.True(t, IsWatchedFile("/x/applet.yaml"))
	assert.True(t, IsWatchedFile("/x/game.js"))
	assert.False(t, IsWatchedFile("/x/matches.log"))
	assert.False(t, IsWatchedFile("/x/.localstorage.json"))
	assert.False(t, IsWatchedFile("/x/run.sh"))
}

func write(t *testing.T, root, id, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(root, id), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, id, FileName), []byte(body), 0o600))
}

func TestStoreKeepsLastGoodAndReportsTheError(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	write(t, root, "thing", minimal)
	require.NoError(t, store.Reload())
	applet, ok := store.Get("thing")
	require.True(t, ok)
	assert.Equal(t, filepath.Join(root, "thing"), applet.Dir)

	write(t, root, "thing", "version: [")
	require.NoError(t, store.Reload())
	entries := onDisk(store)
	require.Len(t, entries, 1)
	require.Error(t, entries[0].Err)
	assert.Equal(t, "Thing", entries[0].Applet.Title)
	_, ok = store.Get("thing")
	assert.True(t, ok, "the last good file keeps running")
}

func TestABrokenFileKeepsTheRevisionSoARunningPageStaysUp(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	write(t, root, "thing", minimal)
	require.NoError(t, store.Reload())
	before := onDisk(store)[0].Revision
	require.NotZero(t, before)

	write(t, root, "thing", "version: [")
	require.NoError(t, store.Reload())
	assert.Equal(t, before, onDisk(store)[0].Revision)
}

func TestRevisionFollowsPagesButNotData(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	write(t, root, "thing", minimal)
	write(t, root, "other", minimal)
	require.NoError(t, store.Reload())
	revisions := func() map[string]int64 {
		out := map[string]int64{}
		for _, entry := range store.List() {
			out[entry.Applet.ID] = entry.Revision
		}
		return out
	}
	before := revisions()

	stamp := func(name string, offset time.Duration) {
		t.Helper()
		path := filepath.Join(root, "thing", name)
		require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
		at := time.UnixMilli(before["thing"]).Add(offset)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	stamp("scores.log", time.Hour)
	require.NoError(t, store.Reload())
	assert.Equal(t, before["thing"], revisions()["thing"], "a data file does not restart the page")

	stamp("game.js", time.Hour)
	require.NoError(t, store.Reload())
	assert.Greater(t, revisions()["thing"], before["thing"])
	assert.Equal(t, before["other"], revisions()["other"], "another applet is left alone")
}

func TestStoreListsAFileThatNeverLoaded(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	write(t, root, "broken", "version: 2")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "unfinished"), 0o755))
	require.NoError(t, store.Reload())

	entries := onDisk(store)
	require.Len(t, entries, 1, "a directory with no applet.yaml is not listed")
	assert.Equal(t, "broken", entries[0].Applet.ID)
	require.Error(t, entries[0].Err)
	_, ok := store.Get("broken")
	assert.False(t, ok, "nothing to run")
}

func TestStoreMissingRootHasOnlyBuiltins(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "absent"))
	require.NoError(t, store.Reload())
	assert.Empty(t, onDisk(store))
	_, ok := store.Get(BuilderID)
	assert.True(t, ok)
}

func TestADirectoryReplacesTheBuiltinOfItsName(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	write(t, root, BuilderID, minimal)
	require.NoError(t, store.Reload())
	applet, ok := store.Get(BuilderID)
	require.True(t, ok)
	assert.False(t, applet.Builtin)
	assert.Equal(t, "Thing", applet.Title)
}

func TestSavedStorageDoesNotHideABuiltin(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	require.NoError(t, os.MkdirAll(filepath.Join(root, BuilderID), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, BuilderID, ".localstorage.json"), []byte("{}"), 0o600))
	require.NoError(t, store.Reload())
	applet, ok := store.Get(BuilderID)
	require.True(t, ok)
	assert.True(t, applet.Builtin)
}

func onDisk(store *Store) []Entry {
	return slices.DeleteFunc(store.List(), func(entry Entry) bool { return entry.Applet.Builtin })
}

func TestParseTitlebar(t *testing.T) {
	t.Run("an applet without it has none", func(t *testing.T) {
		a, err := Parse("thing", []byte(minimal))
		require.NoError(t, err)
		assert.Nil(t, a.Titlebar)
	})
	t.Run("defaults to the applet's page and one icon button", func(t *testing.T) {
		a, err := Parse("thing", []byte(minimal+"\nentry: app.html\ntitlebar: {}\n"))
		require.NoError(t, err)
		assert.Equal(t, &Titlebar{Entry: "app.html", Width: DefaultTitlebarWidth}, a.Titlebar)
	})
	t.Run("keeps its own page and width", func(t *testing.T) {
		a, err := Parse("thing", []byte(minimal+"\ntitlebar:\n  entry: bar.html\n  width: 120\n"))
		require.NoError(t, err)
		assert.Equal(t, &Titlebar{Entry: "bar.html", Width: 120}, a.Titlebar)
	})
	t.Run("refuses a page outside the directory", func(t *testing.T) {
		_, err := Parse("thing", []byte(minimal+"\ntitlebar:\n  entry: ../bar.html\n"))
		assert.ErrorContains(t, err, "titlebar.entry")
	})
	t.Run("refuses a negative width", func(t *testing.T) {
		_, err := Parse("thing", []byte(minimal+"\ntitlebar:\n  width: -1\n"))
		assert.ErrorContains(t, err, "titlebar.width")
	})
}

func TestParseCommands(t *testing.T) {
	applet, err := Parse("radio", []byte(minimal+"commands:\n  - id: next\n    title: Next station\n    icon: play\n    keywords: [skip]\n"))
	require.NoError(t, err)
	assert.Equal(t, []Command{{ID: "next", Title: "Next station", Icon: "play", Keywords: []string{"skip"}}}, applet.Commands)
}
