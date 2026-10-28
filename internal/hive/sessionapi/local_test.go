package sessionapi_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/multiplexer"
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

	sess, err := api.CreateSessionWithWindows(t.Context(), sessionsvc.NewSessionRequest{Name: "spawned", Remote: remote}, nil)
	require.NoError(t, err)
	require.NotNil(t, sess)

	got, err := api.GetSession(t.Context(), sess.ID)
	require.NoError(t, err)
	assert.Equal(t, "spawned", got.Name)
}

// recordingMux records whether a session was created in the background and
// whether anything attached to it.
type recordingMux struct {
	hivetest.Mux
	backgrounds []bool
	attaches    int
}

func (m *recordingMux) CreateSession(_ context.Context, spec multiplexer.SessionSpec) error {
	m.backgrounds = append(m.backgrounds, spec.Background)
	return nil
}

func (m *recordingMux) AttachOrSwitch(context.Context, multiplexer.Target, multiplexer.AttachStreams) error {
	m.attaches++
	return nil
}

func TestLocalCreatesInTheBackgroundWhateverTheCallerAsks(t *testing.T) {
	mux := &recordingMux{}
	api := sessionapi.NewLocal(hivetest.NewEngine(t, hivetest.Options{Mux: mux}).Engine)

	_, err := api.CreateSession(t.Context(), sessionsvc.CreateOptions{Name: "front", Remote: remote, Background: false})
	require.NoError(t, err)
	_, err = api.CreateSessionWithWindows(t.Context(), sessionsvc.NewSessionRequest{Name: "spawned", Remote: remote},
		[]multiplexer.WindowSpec{{Name: "agent", Command: "true"}})
	require.NoError(t, err)

	assert.Equal(t, []bool{true, true}, mux.backgrounds)
	assert.Zero(t, mux.attaches, "the caller attaches, not the API")
}
