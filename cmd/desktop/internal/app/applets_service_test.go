package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
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

func packedApplet(t *testing.T, id, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, applets.FileName), []byte(yaml), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bar.html"), []byte("<!doctype html>"), 0o600))
	var packed bytes.Buffer
	require.NoError(t, applets.Pack(applets.Applet{ID: id, Dir: dir}, &packed))
	return base64.StdEncoding.EncodeToString(packed.Bytes())
}

func TestImportWritesNothingUntilConfirmed(t *testing.T) {
	service := appletHarness(t, "existing", "version: 1\ntitle: Existing\n")
	root := service.catalog.Root()
	preview, err := service.PrepareImport(packedApplet(t, "radio", "version: 1\ntitle: Radio\ntitlebar:\n  entry: bar.html\ncommands:\n  - {id: next, title: Next}\n"))
	require.NoError(t, err)
	assert.Equal(t, "radio", preview.ID)
	assert.Equal(t, "radio", preview.SuggestedID)
	assert.Equal(t, "bar.html", preview.Titlebar.Entry)
	assert.Equal(t, "Next", preview.Commands[0].Title)
	assert.Len(t, preview.Files, 3)
	assert.NoDirExists(t, filepath.Join(root, "radio"))

	service.CancelImport(preview.Token)
	assert.NoDirExists(t, filepath.Join(root, "radio"))
	require.Error(t, service.ConfirmImport(preview.Token, "radio"), "a cancelled import cannot be confirmed")
	assert.NoDirExists(t, filepath.Join(root, "radio"))
}

func TestImportNeedsTheFileThatWasShown(t *testing.T) {
	service := appletHarness(t, "existing", "version: 1\ntitle: Existing\n")
	first, err := service.PrepareImport(packedApplet(t, "first", "version: 1\ntitle: First\n"))
	require.NoError(t, err)
	second, err := service.PrepareImport(packedApplet(t, "second", "version: 1\ntitle: Second\n"))
	require.NoError(t, err)
	require.Error(t, service.ConfirmImport(first.Token, "first"))
	require.NoError(t, service.ConfirmImport(second.Token, "second"))
	_, ok := service.catalog.Get("second")
	assert.True(t, ok)
	require.Error(t, service.ConfirmImport(second.Token, "again"), "a token installs once")
}

func TestImportAsksForAnotherIDAndKeepsWhatIsThere(t *testing.T) {
	service := appletHarness(t, "radio", "version: 1\ntitle: Mine\n")
	preview, err := service.PrepareImport(packedApplet(t, "radio", "version: 1\ntitle: Theirs\n"))
	require.NoError(t, err)
	assert.Equal(t, "radio", preview.ID)
	assert.Equal(t, "radio-2", preview.SuggestedID)

	err = service.ConfirmImport(preview.Token, "radio")
	require.ErrorContains(t, err, "already exists")
	mine, ok := service.catalog.Get("radio")
	require.True(t, ok)
	assert.Equal(t, "Mine", mine.Title)

	require.NoError(t, service.ConfirmImport(preview.Token, preview.SuggestedID))
	theirs, ok := service.catalog.Get("radio-2")
	require.True(t, ok)
	assert.Equal(t, "Theirs", theirs.Title)
}

func TestImportRefusesAHostileFileWithoutWriting(t *testing.T) {
	service := appletHarness(t, "existing", "version: 1\ntitle: Existing\n")
	_, err := service.PrepareImport("not base64!")
	require.Error(t, err)
	_, err = service.PrepareImport(base64.StdEncoding.EncodeToString([]byte("not a zip")))
	require.ErrorContains(t, err, "not a Hive applet file")
	_, err = service.PrepareImport(strings.Repeat("A", applets.MaxArchiveBytes*2))
	require.ErrorContains(t, err, "limit")
}

func TestExportWritesAFileThatImports(t *testing.T) {
	service := appletHarness(t, "radio", "version: 1\ntitle: Radio\n")
	root := service.catalog.Root()
	require.NoError(t, os.WriteFile(filepath.Join(root, "radio", "index.html"), []byte("page"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "radio", ".localstorage.json"), []byte("{}"), 0o600))

	preview, err := service.ExportPreview("radio")
	require.NoError(t, err)
	assert.Equal(t, "radio.hiveapplet", preview.FileName)
	assert.Equal(t, []string{".localstorage.json"}, preview.LeftOut)

	dest := filepath.Join(t.TempDir(), preview.FileName)
	require.NoError(t, service.Export("radio", dest))
	raw, err := os.ReadFile(dest)
	require.NoError(t, err)
	archive, err := applets.Unpack(raw)
	require.NoError(t, err)
	assert.Equal(t, "radio", archive.Applet.ID)
}

func TestExportRefusesABuiltinAndAFailureLeavesNoFile(t *testing.T) {
	service := appletHarness(t, "radio", "version: 1\ntitle: Radio\n")
	_, err := service.ExportPreview(applets.BuilderID)
	require.ErrorContains(t, err, "ships with Hive")

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.hiveapplet")
	require.Error(t, service.Export(applets.BuilderID, dest))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestSharingIsRefusedWhereAppletsDoNotRun(t *testing.T) {
	service := appletHarness(t, "radio", "version: 1\ntitle: Radio\n")
	service.pages = false
	_, err := service.PrepareImport("")
	require.ErrorContains(t, err, "headless")
	_, err = service.ExportPreview("radio")
	require.ErrorContains(t, err, "headless")
}
