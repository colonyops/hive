package review

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	hivedrive "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
)

func repoScope(t *testing.T, dir string) Scope {
	t.Helper()
	space, err := drive.RepositorySpace("acme", "site")
	require.NoError(t, err)
	return Scope{Space: space, Dir: dir}
}

type fakeSource struct {
	spaces []drive.Space
	trees  map[string]drive.Entry
}

func (f fakeSource) Spaces(context.Context) ([]drive.Space, error) { return f.spaces, nil }

func (f fakeSource) Tree(_ context.Context, space drive.Space) (drive.Entry, error) {
	return f.trees[space.Key], nil
}

func planTree(name string) drive.Entry {
	return drive.Entry{Kind: drive.KindDir, Children: []drive.Entry{
		{Path: "plans/" + name, Name: name, Kind: drive.KindMarkdown, Modified: time.Now()},
	}}
}

func sourceFor(t *testing.T, name string) fakeSource {
	t.Helper()
	space := repoScope(t, "").Space
	return fakeSource{spaces: []drive.Space{space}, trees: map[string]drive.Entry{space.Key: planTree(name)}}
}

func TestDriveChangeForAnotherSpaceIsIgnored(t *testing.T) {
	dir := t.TempDir()
	view := New(zerolog.Nop(), nil, sourceFor(t, "a.md"), nil, repoScope(t, dir), nil, nil, 0)

	_, cmd := view.Update(DriveChangedMsg{Payload: events.DriveChangedPayload{Space: "other/repo", Paths: []string{"plans/a.md"}}})
	assert.Nil(t, cmd)
}

func TestDriveChangeForTheViewsSpaceRediscoversDocuments(t *testing.T) {
	dir := t.TempDir()
	view := New(zerolog.Nop(), nil, sourceFor(t, "a.md"), nil, repoScope(t, dir), nil, nil, 0)

	_, cmd := view.Update(DriveChangedMsg{Payload: events.DriveChangedPayload{Space: "acme/site", Paths: []string{"plans/a.md"}}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(DocumentChangeMsg)
	require.True(t, ok)
	require.Len(t, msg.Documents, 1)
	assert.Equal(t, "plans/a.md", msg.Documents[0].RelPath)
}

func TestDriveChangesArriveFromTheBus(t *testing.T) {
	bus := testbus.New(t)
	view := New(zerolog.Nop(), bus.EventBus, nil, nil, repoScope(t, t.TempDir()), nil, nil, 0)
	listen := view.Init()
	require.NotNil(t, listen)

	bus.PublishDriveChanged(events.DriveChangedPayload{Space: "acme/site"})
	msg, ok := listen().(DriveChangedMsg)
	require.True(t, ok)
	assert.Equal(t, "acme/site", msg.Payload.Space)
}

func TestWithoutABusTheViewWaitsForNothing(t *testing.T) {
	view := New(zerolog.Nop(), nil, nil, nil, Scope{}, nil, nil, 0)
	assert.Nil(t, view.Init())
}

func TestSetScopeListsTheNewDirectory(t *testing.T) {
	dir := t.TempDir()
	view := New(zerolog.Nop(), nil, sourceFor(t, "a.md"), nil, Scope{}, nil, nil, 0)
	require.Nil(t, view.SetScope(Scope{}), "the same scope is a no-op")

	cmd := view.SetScope(repoScope(t, dir))
	require.NotNil(t, cmd)
	msg, ok := cmd().(DocumentChangeMsg)
	require.True(t, ok)
	require.Len(t, msg.Documents, 1)
	assert.Equal(t, dir, view.ContextDir())
}

func TestAllScopeListsEverySpaceAndHearsEveryChange(t *testing.T) {
	site := repoScope(t, "").Space
	source := fakeSource{
		spaces: []drive.Space{site, drive.Shared},
		trees:  map[string]drive.Entry{site.Key: planTree("a.md"), drive.SharedKey: planTree("b.md")},
	}
	scope := Scope{All: true, Dir: "/ctx"}
	view := New(zerolog.Nop(), nil, source, nil, scope, nil, nil, 0)

	_, cmd := view.Update(DriveChangedMsg{Payload: events.DriveChangedPayload{Space: "shared"}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(DocumentChangeMsg)
	require.True(t, ok)
	paths := map[string]string{}
	for _, d := range msg.Documents {
		paths[d.RelPath] = d.Path
	}
	assert.Equal(t, map[string]string{
		"acme/site/plans/a.md": "/ctx/acme/site/plans/a.md",
		"shared/plans/b.md":    "/ctx/shared/plans/b.md",
	}, paths)
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("# doc\n"), 0o644))
}

func TestFileInASpaceFolderListsThatFolderOnAChange(t *testing.T) {
	root := t.TempDir()
	svc := hivedrive.NewService(zerolog.Nop(), &config.Config{Context: config.ContextConfig{BaseDir: root}}, nil, nil)
	spaceDir := filepath.Join(root, "acme", "site")
	writeFile(t, filepath.Join(spaceDir, "plans", "x.md"))
	writeFile(t, filepath.Join(spaceDir, "plans", "y.md"))
	writeFile(t, filepath.Join(spaceDir, "research", "z.md"))
	checkout := t.TempDir()
	require.NoError(t, os.Symlink(spaceDir, filepath.Join(checkout, ".hive")))

	scope, doc := FileScope(svc, filepath.Join(checkout, ".hive", "plans", "x.md"), time.Now())
	require.Equal(t, "acme/site", scope.Space.Key)

	view := New(zerolog.Nop(), nil, svc, []Document{doc}, scope, nil, nil, 0)
	_, cmd := view.Update(DriveChangedMsg{Payload: events.DriveChangedPayload{Space: "acme/site"}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(DocumentChangeMsg)
	require.True(t, ok)

	var rels, paths []string
	for _, d := range msg.Documents {
		rels = append(rels, d.RelPath)
		paths = append(paths, d.Path)
		assert.FileExists(t, d.Path)
	}
	assert.ElementsMatch(t, []string{"plans/x.md", "plans/y.md"}, rels)
	assert.Contains(t, paths, doc.Path)
	assert.Equal(t, "plans/x.md", doc.RelPath)
	assert.Equal(t, ".hive/plans/x.md", doc.CheckoutPath(".hive"))
}

func TestFileOutsideEverySpaceIsListedAlone(t *testing.T) {
	svc := hivedrive.NewService(zerolog.Nop(), &config.Config{Context: config.ContextConfig{BaseDir: t.TempDir()}}, nil, nil)
	path := filepath.Join(t.TempDir(), "notes.md")
	writeFile(t, path)

	scope, doc := FileScope(svc, path, time.Now())
	assert.Equal(t, Scope{Dir: filepath.Dir(path)}, scope)
	assert.False(t, scope.InDrive(), "the drive watcher has nothing to refresh")
	assert.Equal(t, path, doc.Path)

	view := New(zerolog.Nop(), nil, svc, []Document{doc}, scope, nil, nil, 0)
	_, cmd := view.Update(DriveChangedMsg{Payload: events.DriveChangedPayload{Space: "acme/site"}})
	assert.Nil(t, cmd)
}

func TestFeedbackNamesTheDocumentThroughTheConfiguredSymlink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plans", "a.md"))
	root := drive.Entry{Kind: drive.KindDir, Children: []drive.Entry{
		{Path: "plans", Kind: drive.KindDir, Children: []drive.Entry{{Path: "plans/a.md", Kind: drive.KindMarkdown}}},
	}}
	docs := DocumentsFromTree(repoScope(t, dir).Space, dir, root, false)
	require.Len(t, docs, 1)

	view := New(zerolog.Nop(), nil, nil, docs, repoScope(t, dir), nil, nil, 0)
	view.SetSymlinkName(".context")
	view.SetSize(100, 30)
	view.LoadDocument(&docs[0])
	view.activeSession = &Session{ID: "s", DocPath: docs[0].Path, Comments: []Comment{
		{ID: "c", StartLine: 1, EndLine: 1, ContextText: "# doc", CommentText: "tighten"},
	}}

	view, _ = view.Update(keyMsg("f"))
	assert.Contains(t, view.feedbackGenerated, ".context/plans/a.md")
	assert.NotContains(t, view.feedbackGenerated, ".hive/")
}
