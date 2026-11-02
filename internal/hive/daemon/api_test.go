package daemon_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/multiplexer"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/daemonclient"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/pkg/executil/executiltest"
)

const remote = "https://github.com/acme/site"

func newClient(t *testing.T) *daemonclient.Client {
	t.Helper()
	d := runDaemon(t, t.TempDir(), false)
	return daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(d.socket), Logger: zerolog.Nop()})
}

func TestClientDrivesTheSessionAPIThroughTheDaemon(t *testing.T) {
	client := newClient(t)
	ctx := t.Context()

	var progress strings.Builder
	created, err := client.CreateSession(ctx, sessionsvc.CreateOptions{Name: "review", Remote: remote, SkipSpawn: true, Progress: &progress})
	require.NoError(t, err)
	assert.Equal(t, "review", created.Name)
	assert.Contains(t, progress.String(), "Session created: review\n")

	listed, err := client.ListSessions(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)
	assert.True(t, created.CreatedAt.Equal(listed[0].CreatedAt))

	require.NoError(t, client.RenameSession(ctx, created.ID, "renamed"))
	require.NoError(t, client.SetSessionGroup(ctx, created.ID, "team"))
	got, err := client.GetSession(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.Name)
	assert.Equal(t, "team", got.Group())

	// The fake git reports a clean checkout with nothing unpushed.
	risk, err := client.CheckSessionRisk(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, sessionsvc.Risk{}, risk)

	changes, err := client.SessionChanges(ctx, created.ID, gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted})
	require.NoError(t, err)
	assert.True(t, changes.Resolved)
	assert.Equal(t, gitstatus.DiffBaseUncommitted, changes.Base)
	assert.Empty(t, changes.Files)

	_, err = client.SessionChanges(ctx, "no-such-session", gitstatus.DiffTarget{Base: gitstatus.DiffBaseBranch})
	require.ErrorIs(t, err, session.ErrNotFound)

	diffs, err := client.SessionFileDiffs(ctx, created.ID, gitstatus.DiffTarget{Base: gitstatus.DiffBaseUncommitted}, []string{"not-changed.txt"})
	require.NoError(t, err)
	assert.Equal(t, []gitstatus.FileDiff{{Path: "not-changed.txt", State: gitstatus.FileDiffStateGone}}, diffs)

	options, err := client.SessionLaunchOptions(ctx)
	require.NoError(t, err)
	assert.Equal(t, "claude", options.DefaultAgent)
	assert.Contains(t, options.Agents, "claude")

	windowed, err := client.CreateSessionWithWindows(ctx, sessionsvc.NewSessionRequest{Name: "spawned", Remote: remote},
		[]multiplexer.WindowSpec{{Name: "agent", Command: "true"}})
	require.NoError(t, err)
	assert.Equal(t, "spawned", windowed.Name)

	statuses, err := client.GitStatuses(ctx, []string{created.ID, "no-such-session"})
	require.NoError(t, err)
	require.Len(t, statuses, 1, "an id with no session is absent")
	assert.True(t, statuses[created.ID].Resolved)
	assert.Equal(t, created.Path, statuses[created.ID].Path)
	assert.Equal(t, "acme", statuses[created.ID].Owner)

	require.NoError(t, client.DeleteSession(ctx, created.ID))
	_, err = client.GetSession(ctx, created.ID)
	require.ErrorIs(t, err, session.ErrNotFound)

	info, err := client.Hello(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test", info.Version)
	health, err := client.Health(ctx)
	require.NoError(t, err)
	assert.True(t, health.DBOK)
	assert.Equal(t, 1, health.SessionsByState[string(session.StateActive)])
}

func TestClientGetsNotFoundForAMissingSession(t *testing.T) {
	_, err := newClient(t).GetSession(t.Context(), "missing")
	require.ErrorIs(t, err, session.ErrNotFound)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestClientEditsTagsThroughTheDaemon(t *testing.T) {
	client := newClient(t)
	ctx := t.Context()
	created, err := client.CreateSession(ctx, sessionsvc.CreateOptions{Name: "tagged", Remote: remote, SkipSpawn: true})
	require.NoError(t, err)

	set, err := client.SetSessionTags(ctx, created.ID, []string{"backend", "urgent"})
	require.NoError(t, err)
	assert.Equal(t, []string{"backend", "urgent"}, set.Tags)

	edited, err := client.EditSessionTags(ctx, created.ID, []string{"review"}, []string{"urgent"})
	require.NoError(t, err)
	assert.Equal(t, []string{"backend", "review"}, edited.Tags)

	got, err := client.GetSession(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, edited.Tags, got.Tags)

	_, err = client.SetSessionTags(ctx, created.ID, []string{"a,b"})
	require.ErrorIs(t, err, session.ErrInvalidTag)
	_, err = client.EditSessionTags(ctx, "missing", []string{"x"}, nil)
	require.ErrorIs(t, err, session.ErrNotFound)
}

func TestCreateWithoutARemoteIsInvalid(t *testing.T) {
	client := newClient(t)
	_, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "x", SkipSpawn: true})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = client.CreateSessionWithWindows(t.Context(), sessionsvc.NewSessionRequest{Name: "y"}, nil)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestConcurrentCreatesKeepTheirOwnProgress(t *testing.T) {
	client := newClient(t)
	names := []string{"alpha", "bravo", "charlie", "delta"}
	progress := make([]strings.Builder, len(names))

	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			_, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: name, Remote: remote, SkipSpawn: true, Progress: &progress[i]})
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	for i, name := range names {
		out := progress[i].String()
		assert.Contains(t, out, "Session created: "+name+"\n")
		for _, other := range names {
			if other != name {
				assert.NotContains(t, out, "Session created: "+other+"\n")
			}
		}
	}
}

func TestCreateFromRequestFailureArrivesAsALaunchError(t *testing.T) {
	exec := &executiltest.Exec{Responses: []executiltest.Response{{Err: errors.New("clone failed"), Stderr: []byte("fatal: repository not found")}}}
	d := runDaemonWith(t, t.TempDir(), daemonOpts{exec: exec})
	client := daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(d.socket), Logger: zerolog.Nop()})

	_, err := client.CreateFromRequest(t.Context(), sessionsvc.LaunchRequest{Name: "draft", Repo: remote})

	launchErr, ok := errors.AsType[*sessionsvc.LaunchError](err)
	require.True(t, ok, "Desktop keeps a failed launch as a draft only when errors.As finds the LaunchError: %v", err)
	assert.Equal(t, "draft", launchErr.Name)
	assert.Equal(t, remote, launchErr.Remote)
	assert.Equal(t, "clone repository", launchErr.Step)
	assert.NotEmpty(t, launchErr.Destination)
}

// recycleOutput streams a line for the recycle commands it recognizes, the
// last one with no newline, so the daemon's final flush is covered.
type recycleOutput struct {
	executiltest.Exec
}

func (e *recycleOutput) RunDirStream(ctx context.Context, dir string, stdout, stderr io.Writer, cmd string, args ...string) error {
	script := strings.Join(args, " ")
	switch {
	case strings.Contains(script, "git fetch"):
		_, _ = io.WriteString(stdout, "From origin\n")
	case strings.Contains(script, "git clean"):
		_, _ = io.WriteString(stdout, "Removing scratch.txt")
	}
	return e.Exec.RunDirStream(ctx, dir, stdout, stderr, cmd, args...)
}

func TestRecycleThroughTheDaemonStreamsProgressAndSucceeds(t *testing.T) {
	d := runDaemonWith(t, t.TempDir(), daemonOpts{exec: &recycleOutput{}})
	client := daemonclient.New(daemonclient.Options{Dialer: daemonclient.UnixDialer(d.socket), Logger: zerolog.Nop()})
	created, err := client.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "reuse", Remote: remote, SkipSpawn: true})
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(created.Path, ".git"), 0o755))

	var progress strings.Builder
	require.NoError(t, client.RecycleSession(t.Context(), created.ID, &progress))

	assert.Contains(t, progress.String(), "From origin\n")
	assert.Contains(t, progress.String(), "Removing scratch.txt\n")
	got, err := client.GetSession(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, session.StateRecycled, got.State)
}
