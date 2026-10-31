package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pageApplet = "version: 1\ntitle: Page\nview: html\n"

const (
	reasonableWait = 5 * time.Second
	pollEvery      = 20 * time.Millisecond
)

func pageHarness(t *testing.T) (*AppletsService, string) {
	t.Helper()
	service := appletHarness(t, "page", pageApplet)
	dir := filepath.Dir(service.List(t.Context())[0].Path)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte(`<!doctype html><html><HEAD lang="en"><title>x</title></HEAD></html>`), 0o600))
	return service, dir
}

func TestAssetInjectsTheBridgeFirstAndCarriesStorage(t *testing.T) {
	service, dir := pageHarness(t)
	require.NoError(t, service.SaveStorage("page", `{"k":"</script><b>"}`))
	asset, err := service.Asset("page", "")
	require.NoError(t, err)
	body := string(asset.Body)
	assert.Contains(t, asset.ContentType, "text/html")
	assert.Less(t, strings.Index(body, "window.hive"), strings.Index(body, "<title>"))
	assert.NotContains(t, body, `"</script><b>"`, "stored text cannot end the script")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "game.js"), []byte("1"), 0o600))
	asset, err = service.Asset("page", "game.js")
	require.NoError(t, err)
	assert.Equal(t, "1", string(asset.Body))

	_, err = service.Asset("page", "../../etc/passwd")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestExecReportsExitCodeAsAResult(t *testing.T) {
	service, dir := pageHarness(t)
	result, err := service.Exec(t.Context(), "page", AppletExec{Command: `cat; echo "$WHO $HIVE_APPLET_DIR" >&2; exit 7`, Stdin: "in", Env: map[string]string{"WHO": "me"}})
	require.NoError(t, err)
	assert.Equal(t, 7, result.Code)
	assert.Equal(t, "in", result.Stdout)
	assert.Equal(t, "me "+dir, strings.TrimSpace(result.Stderr))
}

func TestSpawnStreamsTakesInputAndExits(t *testing.T) {
	service, _ := pageHarness(t)
	ctx := t.Context()
	handle, err := service.Spawn(ctx, "page", AppletExec{Command: `read line; echo "got $line"; exit 3`})
	require.NoError(t, err)
	require.NoError(t, service.SpawnWrite(handle, "hello\n"))

	var stdout strings.Builder
	var out AppletOutput
	for !out.Done {
		out, err = service.SpawnRead(ctx, handle)
		require.NoError(t, err)
		stdout.WriteString(out.Stdout)
	}
	assert.Equal(t, "got hello\n", stdout.String())
	assert.Equal(t, 3, out.Code)

	_, err = service.SpawnRead(ctx, handle)
	assert.Equal(t, KindNotFound, KindOf(err), "a finished process is forgotten")
}

func TestClosingAnAppletStopsItsProcesses(t *testing.T) {
	service, dir := pageHarness(t)
	ctx := t.Context()
	marker := filepath.Join(dir, "started")
	handle, err := service.Spawn(ctx, "page", AppletExec{Command: "touch " + marker + "; sleep 60"})
	require.NoError(t, err)
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, reasonableWait, pollEvery)

	spawn, err := service.spawn(handle)
	require.NoError(t, err)
	service.KillSpawns("page")
	require.Eventually(t, func() bool { _, ready := spawn.drain(); return ready }, reasonableWait, pollEvery)
	_, err = service.spawn(handle)
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestFilesResolveAgainstTheAppletAndAnywhereElse(t *testing.T) {
	service, dir := pageHarness(t)
	require.NoError(t, service.WriteFile("page", "sub/notes.txt", "hi"))
	got, err := service.ReadFile("page", "sub/notes.txt")
	require.NoError(t, err)
	assert.Equal(t, "hi", got)
	assert.FileExists(t, filepath.Join(dir, "sub", "notes.txt"))

	outside := filepath.Join(t.TempDir(), "out.txt")
	require.NoError(t, service.WriteFile("page", outside, "free"))
	got, err = service.ReadFile("page", outside)
	require.NoError(t, err)
	assert.Equal(t, "free", got)
}

func TestBridgeRefusesADeclarativeApplet(t *testing.T) {
	service := appletHarness(t, "thing", "version: 1\ntitle: T\nview: table\ncwd: \"~\"\ndata: {command: \"true\"}\ncolumns: [{key: a}]\n")
	_, err := service.Exec(t.Context(), "thing", AppletExec{Command: "true"})
	assert.Equal(t, KindInvalid, KindOf(err))
}
