package pipelinedb

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppend_ReadFrom_Monotonic(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	var offsets []int64
	for i := range 3 {
		offset, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf(`{"n":%d}`, i)))
		require.NoError(t, err)
		offsets = append(offsets, offset)
	}

	// Offsets are strictly increasing.
	for i := 1; i < len(offsets); i++ {
		assert.Greater(t, offsets[i], offsets[i-1])
	}

	msgs, next, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, offsets[2], next)

	for i, msg := range msgs {
		assert.Equal(t, "source:test", msg.Topic)
		assert.Equal(t, fmt.Sprintf("key-%d", i), msg.Key)
		assert.Equal(t, fmt.Sprintf(`{"n":%d}`, i), string(msg.Payload))
		assert.Positive(t, msg.Ts)
		assert.Equal(t, fmt.Sprintf("%d", offsets[i]), msg.ID)
		assert.Nil(t, msg.Meta, "meta is not persisted in phase 1")
	}

	// Reading from the last offset returns nothing new and leaves nextOffset unchanged.
	msgs, next, err = database.ReadFrom(ctx, offsets[2], 10)
	require.NoError(t, err)
	assert.Empty(t, msgs)
	assert.Equal(t, offsets[2], next)

	// Paged reads resume correctly.
	msgs, next, err = database.ReadFrom(ctx, offsets[0], 1)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "key-1", msgs[0].Key)
	assert.Equal(t, offsets[1], next)
}

func TestAppendIfChanged_DedupesByTopicAndKey(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	first, appended, err := database.AppendIfChanged(ctx, "source:one", "shared", []byte(`{"v":1}`))
	require.NoError(t, err)
	assert.True(t, appended)

	unchanged, appended, err := database.AppendIfChanged(ctx, "source:one", "shared", []byte(`{"v":1}`))
	require.NoError(t, err)
	assert.False(t, appended)
	assert.Zero(t, unchanged)

	otherTopic, appended, err := database.AppendIfChanged(ctx, "source:two", "shared", []byte(`{"v":1}`))
	require.NoError(t, err)
	assert.True(t, appended, "the same key in another topic is a distinct source item")

	changed, appended, err := database.AppendIfChanged(ctx, "source:one", "shared", []byte(`{"v":2}`))
	require.NoError(t, err)
	assert.True(t, appended)
	assert.Greater(t, changed, otherTopic)

	assert.Greater(t, otherTopic, first)
	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, []string{"source:one", "source:two", "source:one"}, []string{msgs[0].Topic, msgs[1].Topic, msgs[2].Topic})
	assert.Equal(t, []byte(`{"v":2}`), []byte(msgs[2].Payload))
}

func TestAppendIfChanged_RollsBackHeadOnAppendFailure(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	_, appended, err := database.AppendIfChanged(ctx, "source:test", "item", []byte(`{"v":1}`))
	require.NoError(t, err)
	require.True(t, appended)

	_, err = database.Conn().ExecContext(ctx, `
		CREATE TRIGGER fail_source_head_update
		BEFORE UPDATE ON source_head
		BEGIN
			SELECT RAISE(ABORT, 'source head write failed');
		END;
	`)
	require.NoError(t, err)

	_, appended, err = database.AppendIfChanged(ctx, "source:test", "item", []byte(`{"v":2}`))
	require.Error(t, err)
	assert.False(t, appended)

	msgs, _, err := database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1, "the event append rolls back when the source head update fails")

	_, err = database.Conn().ExecContext(ctx, `DROP TRIGGER fail_source_head_update`)
	require.NoError(t, err)

	_, appended, err = database.AppendIfChanged(ctx, "source:test", "item", []byte(`{"v":2}`))
	require.NoError(t, err)
	assert.True(t, appended, "a failed append must not advance the source head")

	msgs, _, err = database.ReadFrom(ctx, 0, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, []byte(`{"v":2}`), []byte(msgs[1].Payload))
}

func TestReadForConsumer_ResumesFromPersistedOffset(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.CommitBatch(ctx, CommitBatch{Consumer: "flow-1", UpToOffset: "2"}))

	msgs, err := database.ReadForConsumer(ctx, "flow-1", 500)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "3", msgs[0].ID)
}
