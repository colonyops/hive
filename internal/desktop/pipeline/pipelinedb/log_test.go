package pipelinedb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertEventAt inserts an event_log row with an explicit created_at,
// bypassing Append (which always stamps time.Now()) so retention tests can
// backdate rows deterministically instead of sleeping.
func insertEventAt(t *testing.T, database *DB, topic, key string, payload []byte, createdAt int64) int64 {
	t.Helper()
	row := database.Conn().QueryRowContext(context.Background(),
		`INSERT INTO event_log (topic, key, payload, created_at) VALUES (?, ?, ?, ?) RETURNING "offset"`,
		topic, key, payload, createdAt,
	)
	var offset int64
	require.NoError(t, row.Scan(&offset))
	return offset
}

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

func TestCompact_KeepsLatestPerTopicAndKey(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	// Same key appended three times: only the newest should survive.
	_, err := database.Append(ctx, "source:test", "dup", []byte(`{"v":1}`))
	require.NoError(t, err)
	_, err = database.Append(ctx, "source:test", "dup", []byte(`{"v":2}`))
	require.NoError(t, err)
	newest, err := database.Append(ctx, "source:test", "dup", []byte(`{"v":3}`))
	require.NoError(t, err)

	// A distinct key is untouched by key-compaction.
	other, err := database.Append(ctx, "source:test", "other", []byte(`{"v":"other"}`))
	require.NoError(t, err)

	// A matching key in a different topic has a distinct identity and must
	// survive compaction alongside the source:test row above.
	otherTopic, err := database.Append(ctx, "source:other", "dup", []byte(`{"v":"other-topic"}`))
	require.NoError(t, err)

	// Empty-key rows have no identity to compact against and must survive
	// even when duplicated.
	emptyA, err := database.Append(ctx, "source:test", "", []byte(`{"v":"a"}`))
	require.NoError(t, err)
	emptyB, err := database.Append(ctx, "source:test", "", []byte(`{"v":"b"}`))
	require.NoError(t, err)

	require.NoError(t, database.Compact(ctx))

	msgs, _, err := database.ReadFrom(ctx, 0, 100)
	require.NoError(t, err)

	gotOffsets := make(map[int64]string)
	for _, msg := range msgs {
		var offset int64
		_, err := fmt.Sscanf(msg.ID, "%d", &offset)
		require.NoError(t, err)
		gotOffsets[offset] = msg.Key
	}

	assert.Contains(t, gotOffsets, newest, "latest row for a compacted key should survive")
	assert.Contains(t, gotOffsets, other)
	assert.Contains(t, gotOffsets, otherTopic, "compaction identity includes topic")
	assert.Contains(t, gotOffsets, emptyA, "empty-key rows are exempt from compaction")
	assert.Contains(t, gotOffsets, emptyB, "empty-key rows are exempt from compaction")
	assert.Len(t, msgs, 5, "only one row per non-empty topic/key, plus both empty-key rows")
}

func TestCompact_AgeRetention(t *testing.T) {
	database, err := Open(t.TempDir(), OpenOptions{
		Compact: CompactOptions{MaxAge: time.Minute},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	old := insertEventAt(t, database, "source:test", "old-key", []byte(`{}`), time.Now().Add(-time.Hour).UnixNano())
	fresh, err := database.Append(ctx, "source:test", "fresh-key", []byte(`{}`))
	require.NoError(t, err)

	require.NoError(t, database.Compact(ctx))

	msgs, _, err := database.ReadFrom(ctx, 0, 100)
	require.NoError(t, err)
	require.Len(t, msgs, 1, "only the row within MaxAge should survive")
	assert.Equal(t, fmt.Sprintf("%d", fresh), msgs[0].ID)
	assert.NotEqual(t, fmt.Sprintf("%d", old), msgs[0].ID)
}

func TestCompact_CountRetention(t *testing.T) {
	database, err := Open(t.TempDir(), OpenOptions{
		Compact: CompactOptions{MaxRows: 3},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	var offsets []int64
	for i := range 5 {
		// Distinct keys so key-compaction does not itself trim any rows.
		offset, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
		offsets = append(offsets, offset)
	}

	require.NoError(t, database.Compact(ctx))

	msgs, _, err := database.ReadFrom(ctx, 0, 100)
	require.NoError(t, err)
	require.Len(t, msgs, 3, "count retention should cap at MaxRows")

	var gotIDs []string
	for _, msg := range msgs {
		gotIDs = append(gotIDs, msg.ID)
	}
	assert.Equal(t, []string{
		fmt.Sprintf("%d", offsets[2]),
		fmt.Sprintf("%d", offsets[3]),
		fmt.Sprintf("%d", offsets[4]),
	}, gotIDs, "the three newest rows should survive")
}

func TestReadForConsumer_ResumesFromPersistedOffset(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	for i := range 3 {
		_, err := database.Append(ctx, "source:test", fmt.Sprintf("key-%d", i), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, database.Commit(ctx, "flow-1", 2))

	msgs, err := database.ReadForConsumer(ctx, "flow-1", 500)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "3", msgs[0].ID)
}

func TestCommit_Monotonic(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	// An unknown consumer has no committed offset.
	offset, err := database.ConsumerOffset(ctx, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, int64(0), offset)

	require.NoError(t, database.Commit(ctx, "consumer-a", 5))
	offset, err = database.ConsumerOffset(ctx, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset)

	// A lower offset must not regress the committed checkpoint.
	require.NoError(t, database.Commit(ctx, "consumer-a", 3))
	offset, err = database.ConsumerOffset(ctx, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, int64(5), offset, "commit at a lower offset must be a no-op")

	// A higher offset advances it.
	require.NoError(t, database.Commit(ctx, "consumer-a", 10))
	offset, err = database.ConsumerOffset(ctx, "consumer-a")
	require.NoError(t, err)
	assert.Equal(t, int64(10), offset)

	// Consumers are independent.
	offset, err = database.ConsumerOffset(ctx, "consumer-b")
	require.NoError(t, err)
	assert.Equal(t, int64(0), offset)
}
