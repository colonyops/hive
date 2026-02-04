package stores

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hay-kot/hive/internal/core/review"
	"github.com/hay-kot/hive/internal/data/db"
)

func TestReviewStore(t *testing.T) {
	ctx := context.Background()

	t.Run("create and get session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/test.md"
		contentHash := "abc123"
		session, err := store.CreateSession(ctx, docPath, contentHash)
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		if session.ID == "" {
			t.Error("expected non-empty session ID")
		}
		if session.DocumentPath != docPath {
			t.Errorf("got path %q, want %q", session.DocumentPath, docPath)
		}
		if session.FinalizedAt != nil {
			t.Error("expected new session to not be finalized")
		}

		got, err := store.GetSession(ctx, docPath)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}

		if got.ID != session.ID {
			t.Errorf("got ID %q, want %q", got.ID, session.ID)
		}
		if got.DocumentPath != docPath {
			t.Errorf("got path %q, want %q", got.DocumentPath, docPath)
		}
	})

	t.Run("get session not found", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		_, err = store.GetSession(ctx, "/nonexistent.md")
		if !errors.Is(err, review.ErrSessionNotFound) {
			t.Errorf("got %v, want ErrSessionNotFound", err)
		}
	})

	t.Run("unique document path constraint", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/unique.md"
		_, err = store.CreateSession(ctx, docPath, "test-hash")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		// Attempt to create another session for the same document
		_, err = store.CreateSession(ctx, docPath, "test-hash")
		if err == nil {
			t.Error("expected error when creating duplicate session")
		}
	})

	t.Run("finalize session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/finalize-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		if session.IsFinalized() {
			t.Error("new session should not be finalized")
		}

		err = store.FinalizeSession(ctx, session.ID)
		if err != nil {
			t.Fatalf("FinalizeSession: %v", err)
		}

		got, err := store.GetSession(ctx, docPath)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}

		if !got.IsFinalized() {
			t.Error("session should be finalized")
		}
		if got.FinalizedAt == nil {
			t.Error("expected non-nil FinalizedAt")
		}
	})

	t.Run("delete session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/delete-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		err = store.DeleteSession(ctx, session.ID)
		if err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}

		_, err = store.GetSession(ctx, docPath)
		if !errors.Is(err, review.ErrSessionNotFound) {
			t.Errorf("got %v, want ErrSessionNotFound", err)
		}
	})

	t.Run("save and list comments", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/comments-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

		// Initially no comments
		comments, err := store.ListComments(ctx, session.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 0 {
			t.Errorf("got %d comments, want 0", len(comments))
		}

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
		if err != nil {
			t.Fatalf("SaveComment: %v", err)
		}

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
		if err != nil {
			t.Fatalf("SaveComment: %v", err)
		}

		// List comments - should be sorted by start line
		comments, err = store.ListComments(ctx, session.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 2 {
			t.Errorf("got %d comments, want 2", len(comments))
		}

		// Verify sorting (comment2 should be first)
		if comments[0].StartLine != 5 {
			t.Errorf("first comment start line: got %d, want 5", comments[0].StartLine)
		}
		if comments[1].StartLine != 10 {
			t.Errorf("second comment start line: got %d, want 10", comments[1].StartLine)
		}

		// Verify comment data
		if comments[0].CommentText != comment2.CommentText {
			t.Errorf("got comment text %q, want %q", comments[0].CommentText, comment2.CommentText)
		}
	})

	t.Run("delete comment", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/delete-comment-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

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
		if err != nil {
			t.Fatalf("SaveComment: %v", err)
		}

		err = store.DeleteComment(ctx, comment.ID)
		if err != nil {
			t.Fatalf("DeleteComment: %v", err)
		}

		comments, err := store.ListComments(ctx, session.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 0 {
			t.Errorf("got %d comments after delete, want 0", len(comments))
		}
	})

	t.Run("delete session cascades to comments", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		docPath := "/tmp/cascade-test.md"
		session, err := store.CreateSession(ctx, docPath, "test-hash")
		if err != nil {
			t.Fatalf("CreateSession: %v", err)
		}

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
		if err != nil {
			t.Fatalf("SaveComment: %v", err)
		}

		// Verify comment exists
		comments, err := store.ListComments(ctx, session.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 1 {
			t.Errorf("got %d comments, want 1", len(comments))
		}

		// Delete session
		err = store.DeleteSession(ctx, session.ID)
		if err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}

		// Verify comments were cascaded
		comments, err = store.ListComments(ctx, session.ID)
		if err != nil {
			t.Fatalf("ListComments: %v", err)
		}
		if len(comments) != 0 {
			t.Errorf("got %d comments after session delete, want 0", len(comments))
		}
	})

	t.Run("comments isolated by session", func(t *testing.T) {
		database, err := db.Open(t.TempDir(), db.DefaultOpenOptions())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		defer func() { _ = database.Close() }()

		store := NewReviewStore(database)

		// Create two sessions
		session1, err := store.CreateSession(ctx, "/tmp/doc1.md", "hash1")
		if err != nil {
			t.Fatalf("CreateSession 1: %v", err)
		}

		session2, err := store.CreateSession(ctx, "/tmp/doc2.md", "hash2")
		if err != nil {
			t.Fatalf("CreateSession 2: %v", err)
		}

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
		if err != nil {
			t.Fatalf("SaveComment 1: %v", err)
		}

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
		if err != nil {
			t.Fatalf("SaveComment 2: %v", err)
		}

		// Verify each session only sees its own comments
		comments1, err := store.ListComments(ctx, session1.ID)
		if err != nil {
			t.Fatalf("ListComments 1: %v", err)
		}
		if len(comments1) != 1 {
			t.Errorf("session1: got %d comments, want 1", len(comments1))
		}
		if comments1[0].CommentText != comment1.CommentText {
			t.Errorf("session1: got comment %q, want %q", comments1[0].CommentText, comment1.CommentText)
		}

		comments2, err := store.ListComments(ctx, session2.ID)
		if err != nil {
			t.Fatalf("ListComments 2: %v", err)
		}
		if len(comments2) != 1 {
			t.Errorf("session2: got %d comments, want 1", len(comments2))
		}
		if comments2[0].CommentText != comment2.CommentText {
			t.Errorf("session2: got comment %q, want %q", comments2[0].CommentText, comment2.CommentText)
		}
	})
}
