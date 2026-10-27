package sessionapi_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/hivetest"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
)

const remote = "https://github.com/acme/site"

func TestLocalListAndGetRoundTrip(t *testing.T) {
	api := sessionapi.NewLocal(hivetest.NewEngine(t, hivetest.Options{}).Engine)

	created, err := api.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "review", Remote: remote, SkipSpawn: true})
	require.NoError(t, err)

	listed, err := api.ListSessions(t.Context())
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)

	got, err := api.GetSession(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "review", got.Name)

	_, err = api.GetSession(t.Context(), "missing")
	require.ErrorIs(t, err, session.ErrNotFound)
}

func TestLocalStatusAvailableNeedsAPaneSource(t *testing.T) {
	without := sessionapi.NewLocal(hivetest.NewEngine(t, hivetest.Options{}).Engine)
	assert.False(t, without.StatusAvailable())
	assert.Empty(t, without.FetchBatch(t.Context(), []*session.Session{{ID: "a", State: session.StateActive}}, nil))

	with := sessionapi.NewLocal(hivetest.NewEngine(t, hivetest.Options{PaneSource: hivetest.PaneSource{}}).Engine)
	assert.True(t, with.StatusAvailable())
}

func TestLocalCreateSessionWithWindowsReturnsTheSession(t *testing.T) {
	api := sessionapi.NewLocal(hivetest.NewEngine(t, hivetest.Options{}).Engine)

	sess, err := api.CreateSessionWithWindows(t.Context(), sessionsvc.NewSessionRequest{Name: "spawned", Remote: remote}, nil, true)
	require.NoError(t, err)
	require.NotNil(t, sess)

	got, err := api.GetSession(t.Context(), sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "spawned", got.Name)
}
