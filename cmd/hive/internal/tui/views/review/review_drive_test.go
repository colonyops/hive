package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/hive/events/testbus"
)

func repoScope(t *testing.T, dir string) Scope {
	t.Helper()
	space, err := drive.RepositorySpace("acme", "site")
	require.NoError(t, err)
	return Scope{Space: space, Dir: dir}
}

func writePlan(t *testing.T, dir, name string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plans", name), []byte("# "+name), 0o644))
}

func TestDriveChangeForAnotherSpaceIsIgnored(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, dir, "a.md")
	view := New(zerolog.Nop(), nil, nil, repoScope(t, dir), nil, nil, 0)

	_, cmd := view.Update(DriveChangedMsg{Payload: events.DriveChangedPayload{Space: "other/repo", Paths: []string{"plans/a.md"}}})
	assert.Nil(t, cmd)
}

func TestDriveChangeForTheViewsSpaceRediscoversDocuments(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, dir, "a.md")
	view := New(zerolog.Nop(), nil, nil, repoScope(t, dir), nil, nil, 0)

	_, cmd := view.Update(DriveChangedMsg{Payload: events.DriveChangedPayload{Space: "acme/site", Paths: []string{"plans/a.md"}}})
	require.NotNil(t, cmd)
	msg, ok := cmd().(DocumentChangeMsg)
	require.True(t, ok)
	require.Len(t, msg.Documents, 1)
	assert.Equal(t, "plans/a.md", msg.Documents[0].RelPath)
}

func TestDriveChangesArriveFromTheBus(t *testing.T) {
	bus := testbus.New(t)
	view := New(zerolog.Nop(), bus.EventBus, nil, repoScope(t, t.TempDir()), nil, nil, 0)
	listen := view.Init()
	require.NotNil(t, listen)

	bus.PublishDriveChanged(events.DriveChangedPayload{Space: "acme/site"})
	msg, ok := listen().(DriveChangedMsg)
	require.True(t, ok)
	assert.Equal(t, "acme/site", msg.Payload.Space)
}

func TestWithoutABusTheViewWaitsForNothing(t *testing.T) {
	view := New(zerolog.Nop(), nil, nil, Scope{}, nil, nil, 0)
	assert.Nil(t, view.Init())
}

func TestSetScopeListsTheNewDirectory(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, dir, "a.md")
	view := New(zerolog.Nop(), nil, nil, Scope{}, nil, nil, 0)
	require.Nil(t, view.SetScope(Scope{}), "the same scope is a no-op")

	cmd := view.SetScope(repoScope(t, dir))
	require.NotNil(t, cmd)
	msg, ok := cmd().(DocumentChangeMsg)
	require.True(t, ok)
	require.Len(t, msg.Documents, 1)
	assert.Equal(t, dir, view.ContextDir())
}
