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
	service := newAppletsService(store, func(context.Context) []string { return os.Environ() }, func(_ context.Context, slug string) (string, error) {
		if slug == "gone" {
			return "", Errorf(KindNotFound, "no session")
		}
		return sessionDir, nil
	})
	service.pages = true
	return service
}

func appletOnDisk(t *testing.T, service *AppletsService, id string) Applet {
	t.Helper()
	for _, applet := range service.List(t.Context()) {
		if applet.ID == id {
			return applet
		}
	}
	require.Failf(t, "no applet", "%q is not listed", id)
	return Applet{}
}

func TestAppletCarriesItsDirectoryAndRunsRelativeCwdInIt(t *testing.T) {
	service := appletHarness(t, "thing", "version: 1\ntitle: Thing\ncwd: data\n")
	dir := filepath.Dir(appletOnDisk(t, service, "thing").Path)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "data"), 0o755))

	out, err := service.Exec(t.Context(), "thing", AppletExec{Command: `pwd -P; printf %s "$HIVE_APPLET_DIR"`})
	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(filepath.Join(dir, "data"))
	require.NoError(t, err)
	assert.Equal(t, resolved+"\n"+dir, out.Stdout)
}

func TestAPageFollowsTheSessionAndFallsBackToItsOwnDirectory(t *testing.T) {
	service := appletHarness(t, "thing", "version: 1\ntitle: Thing\n")
	dir := filepath.Dir(appletOnDisk(t, service, "thing").Path)
	ctx := t.Context()

	context, err := service.Context(ctx, "thing", "")
	require.NoError(t, err)
	assert.Equal(t, dir, context.Cwd)

	context, err = service.Context(ctx, "thing", "gone")
	require.NoError(t, err)
	assert.Equal(t, dir, context.Cwd, "a session with no directory here")

	context, err = service.Context(ctx, "thing", "some-session")
	require.NoError(t, err)
	assert.NotEqual(t, dir, context.Cwd)
}

func TestBrokenAppletIsListedAndNotRunnable(t *testing.T) {
	service := appletHarness(t, "thing", "version: 1\ntitle: Thing\nview: table\n")
	applet := appletOnDisk(t, service, "thing")
	assert.False(t, applet.Runnable)
	assert.Contains(t, applet.Error, "view", "the schema that is gone is an unknown key")
}

func TestBuiltinIsListed(t *testing.T) {
	service := appletHarness(t, "thing", "version: 1\ntitle: Thing\n")
	builder := appletOnDisk(t, service, applets.BuilderID)
	assert.True(t, builder.Builtin)
	assert.True(t, builder.Runnable)
	assert.Empty(t, builder.Error)
	assert.False(t, appletOnDisk(t, service, "thing").Builtin)
}
