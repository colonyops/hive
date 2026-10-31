package applets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimal = `
version: 1
title: Thing
view: table
data: {command: "true"}
columns: [{key: name}]
`

func TestExamplesParse(t *testing.T) {
	require.NotEmpty(t, Examples())
	for _, example := range Examples() {
		_, err := Parse(example.ID, []byte(example.YAML))
		assert.NoErrorf(t, err, "example %s", example.ID)
	}
}

func TestParseDefaults(t *testing.T) {
	applet, err := Parse("thing", []byte(minimal+"actions: [{id: go, label: Go, run: x}]\n"))
	require.NoError(t, err)
	assert.Equal(t, "thing", applet.ID)
	assert.Equal(t, "json", applet.Data.Format)
	assert.Equal(t, ModeBackground, applet.Actions[0].Mode)
}

func TestParseRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ id, yaml, want string }{
		"unknown key":    {"thing", minimal + "colour: red\n", "colour"},
		"empty":          {"thing", "", "empty"},
		"bad dir":        {"Bad Name", minimal, "slug"},
		"bad view":       {"thing", "version: 1\ntitle: T\nview: grid\ndata: {command: x}\ncolumns: [{key: a}]\n", "view"},
		"fast refresh":   {"thing", "version: 1\ntitle: T\nview: table\ndata: {command: x, refresh: 10ms}\ncolumns: [{key: a}]\n", "refresh"},
		"no columns":     {"thing", "version: 1\ntitle: T\nview: table\ndata: {command: x}\n", "column"},
		"picker no acts": {"thing", "version: 1\ntitle: T\nview: picker\ndata: {command: x}\ncolumns: [{key: a}]\n", "picker"},
		"bad tone":       {"thing", "version: 1\ntitle: T\nview: table\ndata: {command: x}\ncolumns: [{key: a, tones: {x: red}}]\n", "tone"},
		"reserved key":   {"thing", minimal + "actions: [{id: a, label: A, run: x, key: j}]\n", "key"},
		"dup key":        {"thing", minimal + "actions: [{id: a, label: A, run: x, key: r},{id: b, label: B, run: x, key: r}]\n", "twice"},
		"dup id":         {"thing", minimal + "actions: [{id: a, label: A, run: x},{id: a, label: B, run: x}]\n", "twice"},
		"bad mode":       {"thing", minimal + "actions: [{id: a, label: A, run: x, mode: popup}]\n", "mode"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(tc.id, []byte(tc.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestParseHTML(t *testing.T) {
	applet, err := Parse("page", []byte("version: 1\ntitle: Page\nview: html\n"))
	require.NoError(t, err)
	assert.Equal(t, DefaultEntry, applet.Entry)

	_, err = Parse("page", []byte("version: 1\ntitle: Page\nview: html\nentry: ../outside.html\n"))
	require.Error(t, err)
	_, err = Parse("page", []byte("version: 1\ntitle: Page\nview: html\ncolumns: [{key: a}]\n"))
	require.Error(t, err)
}

func TestWatchedFilesReloadPagesButNotTheirData(t *testing.T) {
	assert.True(t, IsWatchedFile("/x/applet.yaml"))
	assert.True(t, IsWatchedFile("/x/game.js"))
	assert.False(t, IsWatchedFile("/x/matches.log"))
	assert.False(t, IsWatchedFile("/x/.localstorage.json"))
	assert.False(t, IsWatchedFile("/x/run.sh"))
}

func TestParseRows(t *testing.T) {
	for name, tc := range map[string]struct {
		format, stdout string
		want           []Row
	}{
		"array":    {FormatJSON, `[{"a":"x","n":1.50,"b":true,"z":null,"o":{"k":1}}]`, []Row{{"a": "x", "n": "1.50", "b": "true", "z": "", "o": `{"k":1}`}}},
		"jsonl":    {FormatJSON, "{\"a\":\"1\"}\n{\"a\":\"2\"}\n", []Row{{"a": "1"}, {"a": "2"}}},
		"empty":    {FormatJSON, "  \n", []Row{}},
		"lines":    {FormatLines, "one\n\n two\n", []Row{{LineKey: "one"}, {LineKey: " two"}}},
		"no lines": {FormatLines, "", []Row{}},
	} {
		t.Run(name, func(t *testing.T) {
			rows, truncated, err := ParseRows(tc.format, []byte(tc.stdout))
			require.NoError(t, err)
			assert.False(t, truncated)
			assert.Equal(t, tc.want, rows)
		})
	}

	_, _, err := ParseRows(FormatJSON, []byte("not json"))
	require.Error(t, err)
	_, _, err = ParseRows(FormatJSON, []byte(`[1,2]`))
	require.Error(t, err)
}

func TestParseRowsTruncates(t *testing.T) {
	lines := make([]byte, 0, (MaxRows+1)*2)
	for range MaxRows + 1 {
		lines = append(lines, 'x', '\n')
	}
	rows, truncated, err := ParseRows(FormatLines, lines)
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Len(t, rows, MaxRows)
}

func TestRowEnv(t *testing.T) {
	env, err := RowEnv(Row{"Names": "a; rm -rf /", "com.docker-id": "x"})
	require.NoError(t, err)
	assert.Contains(t, env, "HIVE_ROW_NAMES=a; rm -rf /")
	assert.Contains(t, env, "HIVE_ROW_COM_DOCKER_ID=x")
	assert.Contains(t, env, `HIVE_ROW={"Names":"a; rm -rf /","com.docker-id":"x"}`)
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
	entries := store.List()
	require.Len(t, entries, 1)
	require.Error(t, entries[0].Err)
	assert.Equal(t, "Thing", entries[0].Applet.Title)
	_, ok = store.Get("thing")
	assert.True(t, ok, "the last good declaration keeps running")
}

func TestABrokenFileKeepsTheRevisionSoARunningPageStaysUp(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	write(t, root, "thing", minimal)
	require.NoError(t, store.Reload())
	before := store.List()[0].Revision
	require.NotZero(t, before)

	write(t, root, "thing", "version: [")
	require.NoError(t, store.Reload())
	assert.Equal(t, before, store.List()[0].Revision)
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

	entries := store.List()
	require.Len(t, entries, 1, "a directory with no applet.yaml is not listed")
	assert.Equal(t, "broken", entries[0].Applet.ID)
	require.Error(t, entries[0].Err)
	_, ok := store.Get("broken")
	assert.False(t, ok, "nothing to run")
}

func TestStoreMissingRootIsEmpty(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "absent"))
	require.NoError(t, store.Reload())
	assert.Empty(t, store.List())
}
