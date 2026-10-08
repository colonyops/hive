package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
	execsource "github.com/colonyops/hive/cmd/desktop/internal/app/sources/exec"
	"github.com/colonyops/hive/pkg/duration"
)

// The exec connector is the first source whose failures are entirely the
// user's command, so these drive it through the real producer and store rather
// than the connector alone. The property under test is the one the whole design
// rests on: a broken command must not archive the items the source owns.

type execEnvironment struct{}

func (execEnvironment) Environ(context.Context) []string { return nil }

func execInstance(t *testing.T, command string) connector.Instance {
	t.Helper()
	instance, err := execsource.NewFactory(execEnvironment{}).New(
		connector.Node{FlowID: "oncall", NodeID: "src", Policy: models.ResurfacePolicyStateChanges},
		&execsource.Config{Command: command, Timeout: duration.Duration(10 * time.Second)},
	)
	require.NoError(t, err)
	return instance
}

func execSourceKeys(t *testing.T, db *queries.DB) []string {
	t.Helper()
	st := stores.New(db, stores.Options{})
	keys, err := st.SourceHeads.ListActiveKeys(t.Context(), stores.SourceIdentity{
		Topic: "source:oncall/src", ProfileID: "oncall", SourceKind: "exec", SourceScope: "src",
	})
	require.NoError(t, err)
	return keys
}

func TestExecSource_IngestsItsSnapshot(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	sources := stubSources{instances: []connector.Instance{
		execInstance(t, `echo '[{"id":"a","title":"Alpha"},{"id":"b","title":"Beta"}]'`),
	}}

	summary := newTestProducer(db, sources, time.Hour, nil, zerolog.Nop()).Tick(t.Context())

	assert.Zero(t, summary.Failed)
	assert.ElementsMatch(t, []string{"a", "b"}, execSourceKeys(t, db))
}

// The failure this connector's contract exists to prevent: a command that
// breaks must leave the feed alone, not empty it.
func TestExecSource_BrokenCommandKeepsThePreviousSnapshot(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	good := stubSources{instances: []connector.Instance{execInstance(t, `echo '[{"id":"a","title":"Alpha"}]'`)}}
	newTestProducer(db, good, time.Hour, nil, zerolog.Nop()).Tick(t.Context())
	require.Equal(t, []string{"a"}, execSourceKeys(t, db))

	broken := stubSources{instances: []connector.Instance{execInstance(t, `echo "not logged in" >&2; exit 1`)}}
	recorder := &activityRecorder{}
	producer := newTestProducer(db, broken, time.Hour, nil, zerolog.Nop())
	producer.SetRecorder(recorder)

	summary := producer.Tick(t.Context())

	assert.Equal(t, 1, summary.Failed)
	assert.Equal(t, []string{"a"}, execSourceKeys(t, db), "a failed run is not an empty snapshot")
	require.Len(t, recorder.events, 1)
	assert.Contains(t, recorder.events[0].Body, "not logged in")
}

// The counterpart: an empty array really is an empty snapshot, and the
// connector's absence confirmer resolves what left it.
func TestExecSource_EmptyArrayResolvesEveryItem(t *testing.T) {
	t.Parallel()

	db := openTestPipelineDB(t)
	good := stubSources{instances: []connector.Instance{execInstance(t, `echo '[{"id":"a","title":"Alpha"}]'`)}}
	newTestProducer(db, good, time.Hour, nil, zerolog.Nop()).Tick(t.Context())
	require.Equal(t, []string{"a"}, execSourceKeys(t, db))

	empty := stubSources{instances: []connector.Instance{execInstance(t, `echo '[]'`)}}
	newTestProducer(db, empty, time.Hour, nil, zerolog.Nop()).Tick(t.Context())

	assert.Empty(t, execSourceKeys(t, db), "an item that left an authoritative snapshot is gone")
}
