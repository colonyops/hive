package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/applets"
)

func appletHarness(t *testing.T, id, body string) *AppletsService {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, id), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, id, applets.FileName), []byte(body), 0o600))
	store := applets.NewStore(root)
	require.NoError(t, store.Reload())
	sessionDir := t.TempDir()
	return newAppletsService(store, func(context.Context) []string { return os.Environ() }, func(_ context.Context, slug string) (string, error) {
		if slug == "gone" {
			return "", Errorf(KindNotFound, "no session")
		}
		return sessionDir, nil
	})
}

func TestAppletRowsAndActionEnvironment(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out")
	service := appletHarness(t, "thing", `
version: 1
title: Thing
view: table
cwd: "~"
data:
  command: printf '{"name":"a b; touch /tmp/hive-applet-injected"}\n'
columns: [{key: name}]
actions:
  - id: record
    label: Record
    run: printf '%s' "$HIVE_ROW_NAME" > `+out+` && echo done
`)
	ctx := t.Context()

	rows, err := service.Rows(ctx, "thing", "")
	require.NoError(t, err)
	require.Len(t, rows.Rows, 1)

	stdout, err := service.Run(ctx, "thing", "record", "", rows.Rows[0])
	require.NoError(t, err)
	assert.Equal(t, "done", stdout)

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "a b; touch /tmp/hive-applet-injected", string(written), "a row value reaches the command as one word")
	assert.NoFileExists(t, "/tmp/hive-applet-injected")
}

func TestAppletCarriesItsDirectoryAndRunsRelativeCwdInIt(t *testing.T) {
	service := appletHarness(t, "thing", `
version: 1
title: Thing
view: picker
cwd: .
data:
  command: printf '%s\n%s\n' "$PWD" "$HIVE_APPLET_DIR"
  format: lines
columns: [{key: line}]
actions: [{id: a, label: A, run: "true"}]
`)
	rows, err := service.Rows(t.Context(), "thing", "")
	require.NoError(t, err)
	require.Len(t, rows.Rows, 2)
	resolved, err := filepath.EvalSymlinks(rows.Rows[1]["line"])
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(rows.Rows[0]["line"])
	require.NoError(t, err)
	assert.Equal(t, resolved, got)
}

func TestAppletFailureCarriesStderr(t *testing.T) {
	service := appletHarness(t, "thing", `
version: 1
title: Thing
view: table
cwd: "~"
data: {command: "echo daemon is down >&2; exit 3"}
columns: [{key: name}]
`)
	_, err := service.Rows(t.Context(), "thing", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "daemon is down")
	assert.Equal(t, KindUnavailable, KindOf(err))
}

func TestAppletBadOutputIsInvalid(t *testing.T) {
	service := appletHarness(t, "thing", `
version: 1
title: Thing
view: table
cwd: "~"
data: {command: "echo nope"}
columns: [{key: name}]
`)
	_, err := service.Rows(t.Context(), "thing", "")
	require.Error(t, err)
	assert.Equal(t, KindInvalid, KindOf(err))
}

func TestSessionScopedAppletNeedsASession(t *testing.T) {
	service := appletHarness(t, "thing", `
version: 1
title: Thing
view: table
data: {command: "pwd"}
columns: [{key: line}]
`)
	list := service.List(t.Context())
	require.Len(t, list, 1)
	assert.True(t, list[0].RequiresSession)

	_, err := service.Rows(t.Context(), "thing", "")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = service.Rows(t.Context(), "thing", "gone")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestAppletActionsAreResolvedFromTheCatalog(t *testing.T) {
	service := appletHarness(t, "thing", `
version: 1
title: Thing
view: table
cwd: "~"
data: {command: "true"}
columns: [{key: name}]
actions:
  - {id: quiet, label: Quiet, run: "true"}
  - {id: loud, label: Loud, run: top, mode: terminal}
`)
	ctx := t.Context()
	_, err := service.Run(ctx, "thing", "missing", "", nil)
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = service.Run(ctx, "absent", "quiet", "", nil)
	assert.Equal(t, KindNotFound, KindOf(err))
	_, err = service.Run(ctx, "thing", "loud", "", nil)
	assert.Equal(t, KindInvalid, KindOf(err))

	_, err = service.terminalLaunch(ctx, "thing", "quiet", "", nil)
	assert.Equal(t, KindInvalid, KindOf(err))
	launch, err := service.terminalLaunch(ctx, "thing", "loud", "", applets.Row{"name": "x"})
	require.NoError(t, err)
	assert.Equal(t, "top", launch.Command)
	assert.Contains(t, launch.Env, "HIVE_ROW_NAME=x")
}

func TestBrokenAppletIsListedAndNotRunnable(t *testing.T) {
	service := appletHarness(t, "thing", "version: 2\n")
	list := service.List(t.Context())
	require.Len(t, list, 1)
	assert.False(t, list[0].Runnable)
	assert.False(t, list[0].RequiresSession)
	assert.Contains(t, list[0].Error, "version")
	_, err := service.Rows(t.Context(), "thing", "")
	assert.Equal(t, KindNotFound, KindOf(err))
}
