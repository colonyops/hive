package pipelinedb

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrune_EventLogKeepsRowsNotConsumedByEveryEnabledFlow(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for i := range 5 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.CommitBatch(ctx, CommitBatch{Consumer: "fast-flow", UpToOffset: "5"}))
	require.NoError(t, database.CommitBatch(ctx, CommitBatch{Consumer: "slow-flow", UpToOffset: "2"}))

	result, err := database.Prune(ctx, []string{"fast-flow", "slow-flow"}, DefaultRetentionPolicy())
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.EventLogThrough)

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, []string{"3", "4", "5"}, []string{msgs[0].ID, msgs[1].ID, msgs[2].ID},
		"rows after the slow enabled flow's durable offset must remain")
}

func TestPrune_EventLogWaitsForEveryEnabledFlowToCommit(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.CommitBatch(ctx, CommitBatch{Consumer: "committed", UpToOffset: "3"}))

	result, err := database.Prune(ctx, []string{"committed", "newly-enabled"}, DefaultRetentionPolicy())
	require.NoError(t, err)
	assert.Zero(t, result.EventLogThrough)

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	assert.Len(t, msgs, 3, "a flow without a durable offset must retain its full backlog")
}

func TestPrune_BoundsOnlyTerminalHistory(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := database.Conn().ExecContext(ctx, `
			INSERT INTO node_run (flow_id, node_id, ok, in_count, out_count, drop_count, ended_at, dur_ms)
			VALUES ('flow', ?, 1, 0, 0, 0, ?, 0)
		`, fmt.Sprintf("node-%d", i), i+1)
		require.NoError(t, err)
	}
	for i, status := range []string{"done", "failed", "done", "pending", "awaiting_confirmation", "running"} {
		_, err := database.Conn().ExecContext(ctx, `
			INSERT INTO output_command (action_id, key, payload, status, created_at)
			VALUES (?, ?, X'7B7D', ?, ?)
		`, fmt.Sprintf("action-%d", i), fmt.Sprintf("key-%d", i), status, i)
		require.NoError(t, err)
	}

	_, err := database.Prune(ctx, nil, RetentionPolicy{NodeRunLimit: 2, TerminalOutputCommandLimit: 2})
	require.NoError(t, err)

	var nodeRuns int
	require.NoError(t, database.Conn().QueryRowContext(ctx, `SELECT COUNT(*) FROM node_run`).Scan(&nodeRuns))
	assert.Equal(t, 2, nodeRuns)

	var statuses []string
	rows, err := database.Conn().QueryContext(ctx, `SELECT status FROM output_command ORDER BY id`)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	for rows.Next() {
		var status string
		require.NoError(t, rows.Scan(&status))
		statuses = append(statuses, status)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"failed", "done", "pending", "awaiting_confirmation", "running"}, statuses)
}

func TestOpen_FreshDB_HasRetentionIndexes(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for _, index := range []string{
		"idx_node_run_ended_at",
		"idx_node_run_flow_ended_at",
		"idx_output_command_terminal_id",
	} {
		var count int
		require.NoError(t, database.Conn().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index,
		).Scan(&count))
		assert.Equal(t, 1, count, "%s should exist", index)
	}
}
