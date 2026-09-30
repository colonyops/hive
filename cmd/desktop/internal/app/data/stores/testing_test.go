package stores

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

func openTestStores(t *testing.T) (*Stores, *queries.DB) {
	t.Helper()
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return New(db, Options{}), db
}

type testClassifier struct {
	classify func(*models.Observation, models.Observation) models.Classification
}

func (c testClassifier) Classify(prev *models.Observation, current models.Observation) models.Classification {
	return c.classify(prev, current)
}

func activityClassifier(key string) testClassifier {
	return testClassifier{func(_ *models.Observation, _ models.Observation) models.Classification {
		return models.Classification{Kind: "activity", Attention: models.AttentionActivity, Transition: models.TransitionNone, Lifecycle: models.LifecycleActive, OccurrenceKey: key, Summary: "activity"}
	}}
}

type testClock struct{ now time.Time }

func newTestClock() *testClock { return &testClock{now: time.UnixMilli(1)} }

func (c *testClock) Now() time.Time { return c.now }

func (c *testClock) set(unixMilli int64) { c.now = time.UnixMilli(unixMilli) }
