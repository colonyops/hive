package app

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/daemon"
	"github.com/colonyops/hive/internal/hive/daemon/daemontest"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

// runTestDaemon serves a daemon over its own hive.db, so anything the
// SessionsService reads or writes there can only have gone through the
// client.
func runTestDaemon(t *testing.T, mux *fakeMux) *daemonclient.Client {
	t.Helper()
	return daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(runTestDaemonSocket(t, mux)), Logger: zerolog.Nop()})
}

func runTestDaemonSocket(t *testing.T, mux *fakeMux, configure ...func(*daemon.Options)) string {
	t.Helper()
	return daemontest.Socket(t, mux, configure...)
}

func TestSessionsServiceRunsOverTheDaemon(t *testing.T) {
	daemonMux := &fakeMux{}
	client := runTestDaemon(t, daemonMux)
	created, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "review", Remote: "https://github.com/acme/site", SkipSpawn: true})
	require.NoError(t, err)

	local := newHiveHarness(t, engineOptions{})
	tmux := &fakeSessionTmux{}
	jobs := &fakeJobRunner{}
	svc := newSessionsService(SessionsDeps{Hive: local.engine, Sessions: client, Tmux: tmux, Jobs: jobs, Logger: zerolog.Nop()})

	listed, err := svc.ListSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, listed, 1, "the session exists only in the daemon's hive.db")
	assert.Equal(t, created.ID, listed[0].ID)

	renamed, err := svc.RenameSession(t.Context(), created.ID, "renamed")
	require.NoError(t, err)
	assert.Equal(t, "renamed", renamed.Name)
	assert.Equal(t, [][2]string{{"review", "renamed"}}, tmux.renames, "Desktop renames tmux itself first")
	assert.Equal(t, [][2]string{{"review", "renamed"}}, daemonMux.renamed, "the daemon renamed the row and its tmux session")

	_, err = svc.DeleteSession(t.Context(), created.ID)
	require.NoError(t, err)
	require.NoError(t, jobs.err)
	remaining, err := client.ListSessions(t.Context())
	require.NoError(t, err)
	assert.Empty(t, remaining)
	assert.Empty(t, local.mux.renamed, "the local engine saw none of it")
}

func TestSessionsServiceEditsTagsOverTheDaemon(t *testing.T) {
	client := runTestDaemon(t, &fakeMux{})
	created, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "review", Remote: "https://github.com/acme/site", SkipSpawn: true})
	require.NoError(t, err)

	local := newHiveHarness(t, engineOptions{})
	svc := newSessionsService(SessionsDeps{Hive: local.engine, Sessions: client, Tmux: &fakeSessionTmux{}, Jobs: &fakeJobRunner{}, Logger: zerolog.Nop()})

	edited, err := svc.EditSession(t.Context(), created.ID, "review", []string{"backend"})
	require.NoError(t, err)
	assert.Equal(t, []string{"backend"}, edited.Tags)

	edited, err = svc.EditSessionTags(t.Context(), created.ID, []string{"urgent"}, []string{"backend"})
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent"}, edited.Tags)

	stored, err := client.GetSession(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent"}, stored.Tags, "the tags live only in the daemon's hive.db")

	_, err = svc.EditSessionTags(t.Context(), created.ID, []string{"a,b"}, nil)
	assert.Equal(t, KindInvalid, KindOf(err))
}

// oldDaemon answers as a daemon whose hive predates FileContext and FileImage.
type oldDaemon struct {
	sessionapi.API
}

func (oldDaemon) SessionFileContext(context.Context, string, gitstatus.DiffTarget, string) (gitstatus.FileDiff, error) {
	return gitstatus.FileDiff{}, daemonclient.ErrFileContextUnsupported
}

func (oldDaemon) SessionFileImage(context.Context, string, gitstatus.DiffTarget, string) (gitstatus.FileImage, error) {
	return gitstatus.FileImage{}, daemonclient.ErrFileImageUnsupported
}

func TestSessionFileImageReadsALocalSessionWhenTheDaemonIsTooOld(t *testing.T) {
	local := newHiveHarness(t, engineOptions{})
	local.save(t, reviewSession())
	svc := newSessionsService(SessionsDeps{Hive: local.engine, Sessions: oldDaemon{}, Logger: zerolog.Nop()})

	image, err := svc.SessionFileImage(t.Context(), "s1", gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted}, "a.png")
	require.NoError(t, err)
	assert.Equal(t, "a.png", image.Path)

	_, err = svc.SessionFileImage(t.Context(), "elsewhere", gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted}, "a.png")
	require.ErrorIs(t, err, daemonclient.ErrFileImageUnsupported, "a session the engine does not hold keeps the daemon's answer")
}

func TestSessionFileContextReadsALocalSessionWhenTheDaemonIsTooOld(t *testing.T) {
	local := newHiveHarness(t, engineOptions{})
	local.save(t, reviewSession())
	svc := newSessionsService(SessionsDeps{Hive: local.engine, Sessions: oldDaemon{}, Logger: zerolog.Nop()})

	diff, err := svc.SessionFileContext(t.Context(), "s1", gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted}, "a.go")
	require.NoError(t, err)
	assert.Equal(t, "a.go", diff.Path)

	_, err = svc.SessionFileContext(t.Context(), "elsewhere", gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted}, "a.go")
	require.ErrorIs(t, err, daemonclient.ErrFileContextUnsupported, "a session the engine does not hold keeps the daemon's answer")
}
