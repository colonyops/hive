package main

import (
	"testing"

	"github.com/colonyops/hive/internal/desktop/pipeline/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowsServiceSetFlowEnabled(t *testing.T) {
	store := flow.NewFlowStore(t.TempDir(), nil)
	created, err := store.Create("Triage")
	require.NoError(t, err)

	updates := 0
	service := NewFlowsService(store, func() { updates++ })
	summary, err := service.SetFlowEnabled(created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, created.ID, summary.ID)
	assert.False(t, summary.Enabled)
	assert.True(t, summary.Valid)
	assert.Equal(t, 1, updates)

	stored, ok := store.Get(created.ID)
	require.True(t, ok)
	assert.False(t, stored.Enabled)
}

func TestFlowsServiceSetFlowEnabledDoesNotEmitOnFailure(t *testing.T) {
	updates := 0
	service := NewFlowsService(flow.NewFlowStore(t.TempDir(), nil), func() { updates++ })

	_, err := service.SetFlowEnabled("missing", false)
	require.Error(t, err)
	assert.Zero(t, updates)
}
