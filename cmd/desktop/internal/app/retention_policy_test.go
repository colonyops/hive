package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func TestRetentionPolicyAppliesActionRunsOverTheDefaults(t *testing.T) {
	assert.Equal(t, int64(queries.DefaultActionRunLimit), retentionPolicy(settings.RetentionSettings{}).ActionRunLimit)
	assert.Equal(t, int64(7), retentionPolicy(settings.RetentionSettings{ActionRuns: 7}).ActionRunLimit)
	assert.Equal(t, queries.DefaultRetentionPolicy().JobLimit, retentionPolicy(settings.RetentionSettings{ActionRuns: 7}).JobLimit)
}
