package driveremote_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/hive/driveremote"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

func TestResolveThread(t *testing.T) {
	threads := []driveapi.Thread{
		{ID: "019a0000-aaaa-7000-8000-00000000a1b2"},
		{ID: "019a0000-aaaa-7000-8000-00000000c3b2"},
	}

	got, err := driveremote.ResolveThread(threads, "0000a1b2")
	require.NoError(t, err)
	assert.Equal(t, threads[0].ID, got.ID)

	got, err = driveremote.ResolveThread(threads, threads[1].ID)
	require.NoError(t, err)
	assert.Equal(t, threads[1].ID, got.ID)

	_, err = driveremote.ResolveThread(threads, "b2")
	require.ErrorContains(t, err, "matches 2 threads")

	_, err = driveremote.ResolveThread(threads, "ffff")
	require.ErrorContains(t, err, "no thread matches")

	_, err = driveremote.ResolveThread(threads, " ")
	assert.Error(t, err)
}

func TestReplyListsThenActsOnTheDocumentsThread(t *testing.T) {
	f := newFixture(t)
	f.server.threads["plans/ship.md"] = []driveapi.Thread{{ID: "t-0000beef"}}

	c, err := f.svc.Reply(t.Context(), f.server.URL, f.space, "plans/ship.md", "beef", "on it")
	require.NoError(t, err)
	assert.Equal(t, "on it", c.Body)
	assert.Equal(t, []string{"t-0000beef: on it"}, f.server.replies)

	_, err = f.svc.Reply(t.Context(), f.server.URL, f.space, "plans/other.md", "beef", "x")
	require.ErrorContains(t, err, "no thread matches", "a thread on another document is not reached")

	th, err := f.svc.SetResolved(t.Context(), f.server.URL, f.space, "plans/ship.md", "t-0000beef", true)
	require.NoError(t, err)
	assert.NotNil(t, th.ResolvedAt)
}

func TestCommentsOnAnOlderServer(t *testing.T) {
	f := newFixture(t)
	f.server.features = nil

	_, err := f.svc.Threads(t.Context(), f.server.URL, f.space, "plans/ship.md")
	require.ErrorIs(t, err, driveremote.ErrNoComments)
	_, err = f.svc.Reply(t.Context(), f.server.URL, f.space, "plans/ship.md", "x", "y")
	assert.ErrorIs(t, err, driveremote.ErrNoComments)
}
