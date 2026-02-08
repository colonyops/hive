package stores

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hay-kot/hive/internal/core/review"
	"github.com/hay-kot/hive/internal/data/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewStore(t *testing.T) {
	ctx := context.Background()

	t.Run("create and get session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/test.md"
		contentHash := "abc123"
		session, err := store.CreateSession(ctx, docPath, contentHash)
		require.NoError(t, err, "CreateSession")
		assert.NotEmpty(t, session.ID, "expected non-empty session ID")
		assert.Equal(t, docPath, session.DocumentPath)
		assert.Nil(t, session.FinalizedAt, "expected new session to not be finalized")

		got, err := store.GetSession(ctx, docPath)
		require.NoError(t, err, "GetSession")
		assert.Equal(t, session.ID, got.ID)
		assert.Equal(t, docPath, got.DocumentPath)
	})

	t.Run("get session not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		_, err = store.GetSession(ctx, "/nonexistent.md")
		assert.ErrorIs(t, err, review.ErrSessionNotFound, "got %v, want ErrSessionNotFound", err)
	})

	t.Run("unique document path constraint", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/unique.md"
		_, err = store.CreateSession(ctx, docPath, "test-hash")
		require.NoError(t, err, "CreateSession")

		// Attempt to create another session for the same document
		_, err = store.CreateSession(ctx, docPath, "test-hash")
		assert.Error(t, err, "expected error when creating duplicate session")
	})

	t.Run("finalize session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/finalize-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		require.NoError(t, err, "CreateSession")
		assert.False(t, session.IsFinalized(), "new session should not be finalized")

		err = store.FinalizeSession(ctx, session.ID)
		require.NoError(t, err, "FinalizeSession")

		got, err := store.GetSession(ctx, docPath)
		require.NoError(t, err, "GetSession")
		assert.True(t, got.IsFinalized(), "session should be finalized")
		assert.NotNil(t, got.FinalizedAt, "expected non-nil FinalizedAt")
	})

	t.Run("delete session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/delete-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		require.NoError(t, err, "CreateSession")

		err = store.DeleteSession(ctx, session.ID)
		require.NoError(t, err, "DeleteSession")

		_, err = store.GetSession(ctx, docPath)
		assert.ErrorIs(t, err, review.ErrSessionNotFound, "got %v, want ErrSessionNotFound", err)
	})

	t.Run("save and list comments", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/comments-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		require.NoError(t, err, "CreateSession")

		// Initially no comments
		comments, err := store.ListComments(ctx, session.ID)
		require.NoError(t, err, "ListComments")
		assert.Empty(t, comments, "got %d comments, want 0", len(comments))

		// Add first comment
		comment1 := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			StartLine:   10,
			EndLine:     15,
			ContextText: "Some context text",
			CommentText: "This needs improvement",
			CreatedAt:   time.Now(),
		}
		err = store.SaveComment(ctx, comment1)
		require.NoError(t, err, "SaveComment")

		// Add second comment
		comment2 := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			StartLine:   5,
			EndLine:     7,
			ContextText: "Earlier context",
			CommentText: "Fix this typo",
			CreatedAt:   time.Now(),
		}
		err = store.SaveComment(ctx, comment2)
		require.NoError(t, err, "SaveComment")

		// List comments - should be sorted by start line
		comments, err = store.ListComments(ctx, session.ID)
		require.NoError(t, err, "ListComments")
		require.Len(t, comments, 2, "got %d comments, want 2", len(comments))

		// Verify sorting (comment2 should be first)
		assert.Equal(t, 5, comments[0].StartLine, "first comment start line")
		assert.Equal(t, 10, comments[1].StartLine, "second comment start line")

		// Verify comment data
		assert.Equal(t, comment2.CommentText, comments[0].CommentText)
	})

	t.Run("delete comment", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/delete-comment-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		require.NoError(t, err, "CreateSession")

		comment := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			StartLine:   1,
			EndLine:     1,
			ContextText: "test",
			CommentText: "delete me",
			CreatedAt:   time.Now(),
		}
		err = store.SaveComment(ctx, comment)
		require.NoError(t, err, "SaveComment")

		err = store.DeleteComment(ctx, comment.ID)
		require.NoError(t, err, "DeleteComment")

		comments, err := store.ListComments(ctx, session.ID)
		require.NoError(t, err, "ListComments")
		assert.Empty(t, comments, "got %d comments after delete, want 0", len(comments))
	})

	t.Run("delete session cascades to comments", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/cascade-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		require.NoError(t, err, "CreateSession")

		// Add a comment
		comment := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			StartLine:   1,
			EndLine:     1,
			ContextText: "test",
			CommentText: "will be deleted",
			CreatedAt:   time.Now(),
		}
		err = store.SaveComment(ctx, comment)
		require.NoError(t, err, "SaveComment")

		// Verify comment exists
		comments, err := store.ListComments(ctx, session.ID)
		require.NoError(t, err, "ListComments")
		require.Len(t, comments, 1, "got %d comments, want 1", len(comments))

		// Delete session
		err = store.DeleteSession(ctx, session.ID)
		require.NoError(t, err, "DeleteSession")

		// Verify comments were cascaded
		comments, err = store.ListComments(ctx, session.ID)
		require.NoError(t, err, "ListComments")
		assert.Empty(t, comments, "got %d comments after session delete, want 0", len(comments))
	})

	t.Run("comments isolated by session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		// Create two sessions
		session1, err := store.CreateSession(ctx, "/tmp/doc1.md", "hash1")
		require.NoError(t, err, "CreateSession 1")

		session2, err := store.CreateSession(ctx, "/tmp/doc2.md", "hash2")
		require.NoError(t, err, "CreateSession 2")

		// Add comment to first session
		comment1 := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session1.ID,
			StartLine:   1,
			EndLine:     1,
			ContextText: "session1",
			CommentText: "comment for session1",
			CreatedAt:   time.Now(),
		}
		err = store.SaveComment(ctx, comment1)
		require.NoError(t, err, "SaveComment 1")

		// Add comment to second session
		comment2 := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session2.ID,
			StartLine:   1,
			EndLine:     1,
			ContextText: "session2",
			CommentText: "comment for session2",
			CreatedAt:   time.Now(),
		}
		err = store.SaveComment(ctx, comment2)
		require.NoError(t, err, "SaveComment 2")

		// Verify each session only sees its own comments
		comments1, err := store.ListComments(ctx, session1.ID)
		require.NoError(t, err, "ListComments 1")
		require.Len(t, comments1, 1, "session1: got %d comments, want 1", len(comments1))
		assert.Equal(t, comment1.CommentText, comments1[0].CommentText)

		comments2, err := store.ListComments(ctx, session2.ID)
		require.NoError(t, err, "ListComments 2")
		require.Len(t, comments2, 1, "session2: got %d comments, want 1", len(comments2))
		assert.Equal(t, comment2.CommentText, comments2[0].CommentText)
	})

	t.Run("create and get diff session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		sessionName := "feat-auth-vs-main"
		diffContext := "main..feat-auth"
		docPath := "/tmp/diff/test.go"
		contentHash := "diff123"

		session, err := store.CreateDiffSession(ctx, sessionName, diffContext, docPath, contentHash)
		require.NoError(t, err, "CreateDiffSession")
		assert.NotEmpty(t, session.ID, "expected non-empty session ID")
		assert.Equal(t, sessionName, session.SessionName)
		assert.Equal(t, diffContext, session.DiffContext)
		assert.Equal(t, docPath, session.DocumentPath)
		assert.Nil(t, session.FinalizedAt, "expected new session to not be finalized")

		// Get by context
		got, err := store.GetSessionByContext(ctx, sessionName, diffContext)
		require.NoError(t, err, "GetSessionByContext")
		assert.Equal(t, session.ID, got.ID)
		assert.Equal(t, sessionName, got.SessionName)
		assert.Equal(t, diffContext, got.DiffContext)
	})

	t.Run("get diff session by context not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		_, err = store.GetSessionByContext(ctx, "nonexistent", "main..branch")
		assert.ErrorIs(t, err, review.ErrSessionNotFound, "got %v, want ErrSessionNotFound", err)
	})

	t.Run("save and list diff comments with side", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		sessionName := "test-diff"
		diffContext := "main..feature"
		docPath := "/tmp/diff/comments.go"
		session, err := store.CreateDiffSession(ctx, sessionName, diffContext, docPath, "hash1")
		require.NoError(t, err, "CreateDiffSession")

		// Add comment on old side (deletion)
		commentOld := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			StartLine:   10,
			EndLine:     12,
			ContextText: "- old code",
			CommentText: "This was removed",
			CreatedAt:   time.Now(),
			Side:        "old",
		}
		err = store.SaveComment(ctx, commentOld)
		require.NoError(t, err, "SaveComment old")

		// Add comment on new side (addition)
		commentNew := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			StartLine:   15,
			EndLine:     17,
			ContextText: "+ new code",
			CommentText: "This is the new implementation",
			CreatedAt:   time.Now(),
			Side:        "new",
		}
		err = store.SaveComment(ctx, commentNew)
		require.NoError(t, err, "SaveComment new")

		// List comments
		comments, err := store.ListComments(ctx, session.ID)
		require.NoError(t, err, "ListComments")
		require.Len(t, comments, 2, "got %d comments, want 2", len(comments))

		// Verify side field is preserved
		assert.Equal(t, "old", comments[0].Side, "first comment side")
		assert.Equal(t, "new", comments[1].Side, "second comment side")
		assert.Equal(t, commentOld.CommentText, comments[0].CommentText)
		assert.Equal(t, commentNew.CommentText, comments[1].CommentText)
	})

	t.Run("document review comment has empty side", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		require.NoError(t, err, "Open")
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/doc.md"
		session, err := store.CreateSession(ctx, docPath, "hash1")
		require.NoError(t, err, "CreateSession")

		// Add comment without side (document review)
		comment := review.Comment{
			ID:          uuid.NewString(),
			SessionID:   session.ID,
			StartLine:   5,
			EndLine:     10,
			ContextText: "document text",
			CommentText: "Regular document comment",
			CreatedAt:   time.Now(),
			Side:        "",
		}
		err = store.SaveComment(ctx, comment)
		require.NoError(t, err, "SaveComment")

		comments, err := store.ListComments(ctx, session.ID)
		require.NoError(t, err, "ListComments")
		require.Len(t, comments, 1, "got %d comments, want 1", len(comments))
		assert.Equal(t, "", comments[0].Side, "document comment should have empty side")
	})
}
