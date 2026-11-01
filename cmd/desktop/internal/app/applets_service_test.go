package app

import (
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

type fakeAppletPreferences struct {
	disabled  map[string]bool
	forgotten []string
}

func (f *fakeAppletPreferences) AppletDisabled(id string) (bool, error) { return f.disabled[id], nil }

func (f *fakeAppletPreferences) SetAppletDisabled(id string, disabled bool) error {
	f.disabled[id] = disabled
	return nil
}

func (f *fakeAppletPreferences) ForgetApplet(id string) error {
	f.forgotten = append(f.forgotten, id)
	return nil
}

func testAppletDeps(store *applets.Store, sessionDir func(context.Context, string) (string, error)) AppletsDeps {
	return AppletsDeps{
		Catalog:     store,
		Environ:     func(context.Context) []string { return os.Environ() },
		SessionDir:  sessionDir,
		Preferences: &fakeAppletPreferences{disabled: map[string]bool{}},
		Updated:     func() {},
	}
}

func appletHarness(t *testing.T, id, body string) *AppletsService {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, id), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, id, applets.FileName), []byte(body), 0o600))
	store := applets.NewStore(root, "")
	require.NoError(t, store.Reload())
	sessionDir := t.TempDir()
	service := newAppletsService(testAppletDeps(store, func(_ context.Context, slug string) (string, error) {
		if slug == "gone" {
			return "", Errorf(KindNotFound, "no session")
		}
		return sessionDir, nil
	}))
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
	service := appletHarness(t, "thing", "version: 1\ntitle: Thing\ntitlebar: nope\n")
	applet := appletOnDisk(t, service, "thing")
	assert.False(t, applet.Runnable)
	assert.Contains(t, applet.Error, "Titlebar")
}

func TestUnknownKeyListsAWarningAndStillRuns(t *testing.T) {
	service := appletHarness(t, "thing", "version: 1\ntitle: Thing\nsidebar: {entry: side.html}\n")
	applet := appletOnDisk(t, service, "thing")
	assert.True(t, applet.Runnable)
	assert.Empty(t, applet.Error)
	assert.Equal(t, []string{`this version of Hive ignores "sidebar"`}, applet.Warnings)
}

func TestDisabledAppletIsListedAndServesNothing(t *testing.T) {
	service := appletHarness(t, "thing", "version: 1\ntitle: Thing\n")
	require.NoError(t, os.WriteFile(filepath.Join(service.catalog.Root(), "thing", "index.html"), []byte("<p>hi"), 0o600))
	updates := 0
	service.updated = func() { updates++ }
	assert.True(t, appletOnDisk(t, service, "thing").Enabled)

	require.NoError(t, service.SetEnabled("thing", false))
	assert.Equal(t, 1, updates)
	assert.False(t, appletOnDisk(t, service, "thing").Enabled)
	_, err := service.Asset("thing", "index.html")
	require.ErrorContains(t, err, "turned off")
	_, err = service.BridgeToken("thing")
	require.ErrorContains(t, err, "turned off")
	_, err = service.Exec(t.Context(), "thing", AppletExec{Command: "true"})
	require.ErrorContains(t, err, "turned off")

	require.NoError(t, service.SetEnabled("thing", true))
	_, err = service.Asset("thing", "index.html")
	require.NoError(t, err)
	require.Error(t, service.SetEnabled("absent", true))
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
	packed, err := applets.Pack(applets.Applet{ID: id, Dir: dir}, "")
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(packed.Data)
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
	require.NoError(t, service.Export(preview.Token, dest))
	raw, err := os.ReadFile(dest)
	require.NoError(t, err)
	archive, err := applets.Unpack(raw, "")
	require.NoError(t, err)
	assert.Equal(t, "radio", archive.Applet.ID)
}

func TestExportRefusesABuiltinAndAFailureLeavesNoFile(t *testing.T) {
	service := appletHarness(t, "radio", "version: 1\ntitle: Radio\n")
	_, err := service.ExportPreview(applets.BuilderID)
	require.ErrorContains(t, err, "ships with Hive")

	dir := t.TempDir()
	dest := filepath.Join(dir, "out.hiveapplet")
	require.Error(t, service.Export("not-a-token", dest))
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

func TestExportWritesWhatThePreviewListedAndNothingAddedAfter(t *testing.T) {
	service := appletHarness(t, "radio", "version: 1\ntitle: Radio\n")
	dir := filepath.Join(service.catalog.Root(), "radio")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<p>hi"), 0o600))
	preview, err := service.ExportPreview("radio")
	require.NoError(t, err)

	// A running page writes while the save dialog is open.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "session-dump.json"), []byte("secret"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("changed"), 0o600))

	dest := filepath.Join(t.TempDir(), "radio.hiveapplet")
	require.NoError(t, service.Export(preview.Token, dest))
	raw, err := os.ReadFile(dest)
	require.NoError(t, err)
	archive, err := applets.Unpack(raw, "")
	require.NoError(t, err)
	paths := make([]string, 0, len(archive.Files))
	for _, file := range archive.Files {
		paths = append(paths, file.Path)
		assert.NotEqual(t, "changed", string(file.Data))
	}
	assert.ElementsMatch(t, []string{"applet.yaml", "index.html"}, paths)

	require.ErrorContains(t, service.Export(preview.Token, dest), "no longer waiting", "a token works once")

	preview, err = service.ExportPreview("radio")
	require.NoError(t, err)
	service.CancelExport(preview.Token)
	require.ErrorContains(t, service.Export(preview.Token, dest), "no longer waiting")
}

func TestImportForgetsTheIDOnlyOnceInstalled(t *testing.T) {
	service := appletHarness(t, "existing", "version: 1\ntitle: Existing\n")
	preferences := &fakeAppletPreferences{disabled: map[string]bool{}}
	service.preferences = preferences
	preview, err := service.PrepareImport(packedApplet(t, "radio", "version: 1\ntitle: Radio\n"))
	require.NoError(t, err)

	require.Error(t, service.ConfirmImport("wrong-token", "radio"))
	require.Error(t, service.ConfirmImport(preview.Token, "existing"))
	assert.Empty(t, preferences.forgotten, "an import that failed takes nothing from the id")

	require.NoError(t, service.ConfirmImport(preview.Token, "radio"))
	assert.Equal(t, []string{"radio"}, preferences.forgotten)
}

func TestImportPreviewCarriesOnlyPlainWarnings(t *testing.T) {
	service := appletHarness(t, "existing", "version: 1\ntitle: Existing\n")
	preview, err := service.PrepareImport(packedApplet(t, "radio", "version: 1\ntitle: Radio\nsidebar: {}\nicon: laser\n\"a\\u202Eb\": 1\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{
		`this version of Hive ignores "a key with an unusual name"`,
		`this version of Hive ignores "sidebar"`,
		`this version of Hive has no icon "laser", so it shows the default`,
	}, preview.Warnings)
}
