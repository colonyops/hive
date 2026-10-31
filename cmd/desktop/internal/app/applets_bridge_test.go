package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
	"github.com/colonyops/hive/internal/domain/session"
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
	assert.Contains(t, body, `{"k":"`, "storage is carried")

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

func TestServerBuildServesNoPages(t *testing.T) {
	service, _ := pageHarness(t)
	service.pages = false
	ctx := t.Context()

	_, err := service.Asset("page", "")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = service.Exec(ctx, "page", AppletExec{Command: "true"})
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = service.Spawn(ctx, "page", AppletExec{Command: "true"})
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = service.BridgeToken("page")
	assert.Equal(t, KindUnavailable, KindOf(err))
	_, err = service.ReadFile("page", "index.html")
	assert.Equal(t, KindUnavailable, KindOf(err))
	assert.Equal(t, KindUnavailable, KindOf(service.WriteFile("page", "x", "y")))
	_, err = service.Context(ctx, "page", "")
	assert.Equal(t, KindUnavailable, KindOf(err))

	list := service.List(ctx)
	require.Len(t, list, 1)
	assert.False(t, list[0].Runnable)
	assert.Equal(t, appletPagesUnavailable, list[0].Error)
}

func TestAPageCarriesTheBridgeTokenAndOtherFilesDoNot(t *testing.T) {
	service, dir := pageHarness(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "game.js"), []byte("1"), 0o600))
	token, err := service.BridgeToken("page")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	again, err := service.BridgeToken("page")
	require.NoError(t, err)
	assert.Equal(t, token, again, "a second page of the applet holds the same token")

	page, err := service.Asset("page", "")
	require.NoError(t, err)
	assert.True(t, page.HoldsToken)
	assert.Contains(t, string(page.Body), token)

	script, err := service.Asset("page", "game.js")
	require.NoError(t, err)
	assert.False(t, script.HoldsToken)
}

func TestAssetRefusesDotfiles(t *testing.T) {
	service, _ := pageHarness(t)
	require.NoError(t, service.SaveStorage("page", `{"secret":"1"}`))
	_, err := service.Asset("page", ".localstorage.json")
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = service.Asset("page", "sub/.hidden/file.js")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestKillingASpawnLetsItsTrapRunAndReportsNoError(t *testing.T) {
	service, dir := pageHarness(t)
	ctx := t.Context()
	ready := filepath.Join(dir, "ready")
	cleaned := filepath.Join(dir, "cleaned")
	handle, err := service.Spawn(ctx, "page", AppletExec{
		Command: `trap 'touch ` + cleaned + `; exit 0' TERM; touch ` + ready + `; while :; do sleep 0.05; done`,
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { _, err := os.Stat(ready); return err == nil }, reasonableWait, pollEvery)

	require.NoError(t, service.SpawnKill(handle))

	var stderr strings.Builder
	var out AppletOutput
	for !out.Done {
		out, err = service.SpawnRead(ctx, handle)
		require.NoError(t, err)
		stderr.WriteString(out.Stderr)
	}
	assert.FileExists(t, cleaned, "the shell got SIGTERM and a chance to clean up")
	assert.NotContains(t, stderr.String(), context.Canceled.Error(), "being stopped is not an error to report")
}

func TestASpawnOutlivesTheCallThatStartedIt(t *testing.T) {
	service, _ := pageHarness(t)
	callCtx, finish := context.WithCancel(t.Context())
	handle, err := service.Spawn(callCtx, "page", AppletExec{Command: `read line; echo "got $line"`})
	require.NoError(t, err)
	finish()

	require.NoError(t, service.SpawnWrite(handle, "still here\n"))
	var stdout strings.Builder
	var out AppletOutput
	for !out.Done {
		out, err = service.SpawnRead(t.Context(), handle)
		require.NoError(t, err)
		stdout.WriteString(out.Stdout)
	}
	assert.Equal(t, "got still here\n", stdout.String())
}

func TestPopupOpensAnAppletActionFromTheCatalog(t *testing.T) {
	out := filepath.Join(t.TempDir(), "row")
	cwd := t.TempDir()
	service := appletHarness(t, "thing", `
version: 1
title: Thing
view: table
cwd: `+cwd+`
data: {command: "true"}
columns: [{key: name}]
actions:
  - id: loud
    label: Loud
    mode: terminal
    run: printf '%s' "$HIVE_ROW_NAME" > `+out+`
`)
	popup := newPopupHarness(t, checkoutHarness(t, t.TempDir(), session.StateActive))
	popup.applets = service

	term, err := popup.Open(t.Context(), OpenPopupTerminal{
		Applet:       "thing",
		AppletAction: "loud",
		AppletRow:    applets.Row{"name": "a b; c"},
		Command:      "echo from-the-client",
		Dir:          t.TempDir(),
		Cols:         80,
		Rows:         24,
	})
	require.NoError(t, err)
	assert.Contains(t, term.Command, "HIVE_ROW_NAME", "the catalog's command, not the client's")
	assert.Equal(t, cwd, term.Dir)
	require.Eventually(t, func() bool {
		written, err := os.ReadFile(out)
		return err == nil && string(written) == "a b; c"
	}, reasonableWait, pollEvery, "the row reached the terminal's environment")
}
