package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/store/db"
)

func diffReviewFixture(t *testing.T) (*DiffReviewStore, *SessionStore) {
	t.Helper()
	database := db.OpenForTest(t)
	sessions := NewSessionStore(database)
	now := time.Now()
	for _, id := range []string{"s1", "s2"} {
		require.NoError(t, sessions.Save(t.Context(), session.Session{
			ID: id, Name: id, Slug: id, Path: "/tmp/" + id, State: session.StateActive, CreatedAt: now, UpdatedAt: now,
		}))
	}
	return NewDiffReviewStore(database), sessions
}

func lineComment(id, path string, line int, text string) review.DiffReviewComment {
	return review.DiffReviewComment{
		ID: id, Path: path, StartSide: review.DiffSideAdditions, StartLine: line,
		EndSide: review.DiffSideAdditions, EndLine: line, Quote: "+x", Text: text, Severity: review.SeverityIssue,
	}
}

func TestDiffReviewStoreReplaceKeepsOneReviewPerAuthor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, _ := diffReviewFixture(t)

	require.NoError(t, store.ReplaceDiffReview(ctx, review.DiffReview{
		ID: "r1", SessionID: "s1", Author: "claude", Agent: "claude", Head: "abc", Summary: "first",
		CreatedAt: time.Unix(1, 0), Comments: []review.DiffReviewComment{lineComment("c1", "a.go", 3, "one"), lineComment("c2", "a.go", 9, "two")},
	}))
	require.NoError(t, store.ReplaceDiffReview(ctx, review.DiffReview{
		ID: "r2", SessionID: "s1", Author: "codex", Agent: "codex", Head: "abc",
		CreatedAt: time.Unix(2, 0), Comments: []review.DiffReviewComment{lineComment("c3", "b.go", 1, "three")},
	}))
	require.NoError(t, store.ReplaceDiffReview(ctx, review.DiffReview{
		ID: "r3", SessionID: "s1", Author: "claude", Agent: "claude", Head: "def", Summary: "second",
		CreatedAt: time.Unix(3, 0), Comments: []review.DiffReviewComment{
			{ID: "c4", Kind: review.DiffCommentFile, Path: "c.go", Text: "file", Severity: review.SeverityNote},
			lineComment("c5", "a.go", 4, "four"),
		},
	}))

	got, err := store.ListDiffReviews(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "codex", got[0].Author)
	assert.Equal(t, "r3", got[1].ID)
	assert.Equal(t, "second", got[1].Summary)
	assert.Equal(t, "def", got[1].Head)
	require.Len(t, got[1].Comments, 2)
	assert.Equal(t, "c4", got[1].Comments[0].ID, "comments keep the order they were written in")
	assert.Equal(t, review.DiffCommentFile, got[1].Comments[0].Kind)
	assert.Equal(t, 4, got[1].Comments[1].StartLine)

	other, err := store.ListDiffReviews(ctx, "s2")
	require.NoError(t, err)
	assert.Empty(t, other)
}

func TestDiffReviewStoreDeleteLastCommentDropsReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, _ := diffReviewFixture(t)
	require.NoError(t, store.ReplaceDiffReview(ctx, review.DiffReview{
		ID: "r1", SessionID: "s1", Author: "claude", CreatedAt: time.Unix(1, 0),
		Comments: []review.DiffReviewComment{lineComment("c1", "a.go", 3, "one"), lineComment("c2", "a.go", 9, "two")},
	}))

	require.NoError(t, store.DeleteDiffReviewComment(ctx, "c1"))
	got, err := store.ListDiffReviews(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Len(t, got[0].Comments, 1)

	require.NoError(t, store.DeleteDiffReviewComment(ctx, "c2"))
	got, err = store.ListDiffReviews(ctx, "s1")
	require.NoError(t, err)
	assert.Empty(t, got)

	require.ErrorIs(t, store.DeleteDiffReviewComment(ctx, "c2"), review.ErrDiffReviewNotFound)
	assert.ErrorIs(t, store.DeleteDiffReview(ctx, "r1"), review.ErrDiffReviewNotFound)
}

func TestDiffReviewStoreFollowsTheSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, sessions := diffReviewFixture(t)
	require.NoError(t, store.ReplaceDiffReview(ctx, review.DiffReview{
		ID: "r1", SessionID: "s1", Author: "claude", CreatedAt: time.Unix(1, 0),
		Comments: []review.DiffReviewComment{lineComment("c1", "a.go", 3, "one")},
	}))
	before, err := store.DiffReviewFingerprint(ctx)
	require.NoError(t, err)
	assert.Equal(t, review.DiffReviewFingerprint{Reviews: 1, Comments: 1, Latest: time.Unix(1, 0).UnixNano()}, before)

	require.NoError(t, sessions.Delete(ctx, "s1"))

	got, err := store.ListDiffReviews(ctx, "s1")
	require.NoError(t, err)
	assert.Empty(t, got)
	after, err := store.DiffReviewFingerprint(ctx)
	require.NoError(t, err)
	assert.Equal(t, review.DiffReviewFingerprint{}, after)
}

func TestDiffReviewStoreRejectsAnUnknownSession(t *testing.T) {
	t.Parallel()
	store, _ := diffReviewFixture(t)
	err := store.ReplaceDiffReview(context.Background(), review.DiffReview{ID: "r1", SessionID: "nope", Author: "claude", CreatedAt: time.Now()})
	assert.Error(t, err)
}

func TestDiffReviewStoreDeleteBySession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, _ := diffReviewFixture(t)
	for i, author := range []string{"claude", "codex"} {
		require.NoError(t, store.ReplaceDiffReview(ctx, review.DiffReview{
			ID: author, SessionID: "s1", Author: author, CreatedAt: time.Unix(int64(i), 0),
		}))
	}
	n, err := store.DeleteDiffReviews(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}
