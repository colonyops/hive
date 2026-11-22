package userdb_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
)

var (
	ada   = core.Actor{UserID: "ada", Email: "ada@example.com"}
	bob   = core.Actor{UserID: "bob", Email: "bob@example.com", ViaToken: "laptop"}
	noCap = core.CommentCaps{}
)

type threadFixture struct {
	docs    core.DocumentStore
	threads core.ThreadStore
	doc     core.Document
}

func newThreadFixture(t *testing.T) threadFixture {
	t.Helper()
	p, _ := newPool(t, 4, 0)
	user := uuid.NewString()
	docs, release, err := p.Stores().Open(t.Context(), user)
	require.NoError(t, err)
	t.Cleanup(release)
	threads, releaseThreads, err := p.Stores().OpenThreads(t.Context(), user)
	require.NoError(t, err)
	t.Cleanup(releaseThreads)
	doc, err := docs.Put(t.Context(), core.NewDocument{
		ID: uuid.NewString(), Space: "shared", Path: "plan.md", Kind: drive.KindMarkdown,
		SHA256: "v1", Body: []byte("# Plan\n\none\ntwo\n"), UpdatedAt: time.Now(),
	}, nil)
	require.NoError(t, err)
	return threadFixture{docs: docs, threads: threads, doc: doc}
}

func (f threadFixture) newThread(t *testing.T, anchor *core.ThreadAnchor, body string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, f.threads.InsertThread(t.Context(), core.NewThread{
		ID: id, DocumentID: f.doc.ID, Anchor: anchor, Author: ada,
		CommentID: uuid.NewString(), Body: body, CreatedAt: time.Now(),
	}, "v1", noCap))
	return id
}

func (f threadFixture) reply(t *testing.T, threadID string, by core.Actor, body string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, f.threads.InsertComment(t.Context(), core.NewComment{
		ID: id, ThreadID: threadID, Author: by, Body: body, CreatedAt: time.Now(),
	}, noCap))
	return id
}

func TestThreadsListGeneralFirstThenByLine(t *testing.T) {
	f := newThreadFixture(t)
	late := f.newThread(t, &core.ThreadAnchor{StartLine: 4, EndLine: 4, Quote: "two"}, "on two")
	general := f.newThread(t, nil, "overall")
	early := f.newThread(t, &core.ThreadAnchor{StartLine: 3, EndLine: 3, Quote: "one"}, "on one")
	f.reply(t, early, bob, "agreed")

	got, err := f.threads.ListThreads(t.Context(), f.doc.ID)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{general, early, late}, []string{got[0].ID, got[1].ID, got[2].ID})
	assert.Nil(t, got[0].Anchor)
	require.Len(t, got[1].Comments, 2)
	assert.Equal(t, bob, got[1].Comments[1].Author)
	assert.Equal(t, "one", got[1].Anchor.Quote)
}

func TestInsertThreadRefusesAnotherVersion(t *testing.T) {
	f := newThreadFixture(t)
	err := f.threads.InsertThread(t.Context(), core.NewThread{
		ID: uuid.NewString(), DocumentID: f.doc.ID, Author: ada, CommentID: uuid.NewString(), Body: "x", CreatedAt: time.Now(),
	}, "v0", noCap)
	require.ErrorIs(t, err, core.ErrVersionChanged)

	err = f.threads.InsertThread(t.Context(), core.NewThread{
		ID: uuid.NewString(), DocumentID: uuid.NewString(), Author: ada, CommentID: uuid.NewString(), Body: "x", CreatedAt: time.Now(),
	}, "v1", noCap)
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestCommentCaps(t *testing.T) {
	f := newThreadFixture(t)
	thread := f.newThread(t, nil, "12345")

	insert := func(caps core.CommentCaps, body string) error {
		return f.threads.InsertComment(t.Context(), core.NewComment{
			ID: uuid.NewString(), ThreadID: thread, Author: bob, Body: body, CreatedAt: time.Now(),
		}, caps)
	}
	require.True(t, core.IsKind(insert(core.CommentCaps{CommentsPerThread: 1}, "x"), core.KindTooLarge))
	require.True(t, core.IsKind(insert(core.CommentCaps{BodyBytesPerDocument: 7}, "abc"), core.KindTooLarge))
	require.NoError(t, insert(core.CommentCaps{BodyBytesPerDocument: 8}, "abc"))

	err := f.threads.InsertThread(t.Context(), core.NewThread{
		ID: uuid.NewString(), DocumentID: f.doc.ID, Author: ada, CommentID: uuid.NewString(), Body: "x", CreatedAt: time.Now(),
	}, "v1", core.CommentCaps{ThreadsPerDocument: 1})
	require.True(t, core.IsKind(err, core.KindTooLarge))
}

func TestDeleteCommentTombstonesTheFirstAndDropsTheEmptyThread(t *testing.T) {
	f := newThreadFixture(t)
	thread := f.newThread(t, nil, "first")
	stored, err := f.threads.GetThread(t.Context(), thread)
	require.NoError(t, err)
	first := stored.Comments[0].ID
	second := f.reply(t, thread, bob, "second")

	deleted, err := f.threads.DeleteComment(t.Context(), first, time.Now())
	require.NoError(t, err)
	assert.False(t, deleted)
	stored, err = f.threads.GetThread(t.Context(), thread)
	require.NoError(t, err)
	require.Len(t, stored.Comments, 2)
	assert.NotNil(t, stored.Comments[0].DeletedAt)
	assert.Empty(t, stored.Comments[0].Body)
	assert.Equal(t, ada, stored.Author, "the thread keeps its author")
	require.ErrorIs(t, f.threads.UpdateComment(t.Context(), first, "back", time.Now()), core.ErrNotFound)

	deleted, err = f.threads.DeleteComment(t.Context(), second, time.Now())
	require.NoError(t, err)
	assert.True(t, deleted)
	_, err = f.threads.GetThread(t.Context(), thread)
	require.ErrorIs(t, err, core.ErrNotFound)
}

func TestDeleteReplyKeepsThread(t *testing.T) {
	f := newThreadFixture(t)
	thread := f.newThread(t, nil, "first")
	reply := f.reply(t, thread, bob, "second")

	deleted, err := f.threads.DeleteComment(t.Context(), reply, time.Now())
	require.NoError(t, err)
	assert.False(t, deleted)
	stored, err := f.threads.GetThread(t.Context(), thread)
	require.NoError(t, err)
	assert.Len(t, stored.Comments, 1)
}

func TestSetResolvedAndReopen(t *testing.T) {
	f := newThreadFixture(t)
	thread := f.newThread(t, nil, "first")
	at := time.Unix(1_700_000_000, 0).UTC()

	require.NoError(t, f.threads.SetResolved(t.Context(), thread, &bob, at))
	stored, err := f.threads.GetThread(t.Context(), thread)
	require.NoError(t, err)
	require.NotNil(t, stored.ResolvedAt)
	assert.Equal(t, at, *stored.ResolvedAt)
	assert.Equal(t, &bob, stored.ResolvedBy)

	require.NoError(t, f.threads.SetResolved(t.Context(), thread, nil, at))
	stored, err = f.threads.GetThread(t.Context(), thread)
	require.NoError(t, err)
	assert.Nil(t, stored.ResolvedAt)
	assert.Nil(t, stored.ResolvedBy)

	require.ErrorIs(t, f.threads.SetResolved(t.Context(), uuid.NewString(), nil, at), core.ErrNotFound)
}

func TestMoveAnchorsOnlyMovesThreadsOnTheOldVersion(t *testing.T) {
	f := newThreadFixture(t)
	moved := f.newThread(t, &core.ThreadAnchor{StartLine: 3, EndLine: 3, Quote: "one"}, "a")
	f.newThread(t, nil, "general threads are never anchored")

	anchored, err := f.threads.AnchoredAt(t.Context(), f.doc.ID, "v1")
	require.NoError(t, err)
	require.Len(t, anchored, 1)
	assert.Equal(t, moved, anchored[0].ThreadID)

	next := core.ThreadAnchor{ThreadID: moved, StartLine: 5, EndLine: 6, Quote: "one", Outdated: true}
	require.NoError(t, f.threads.MoveAnchors(t.Context(), f.doc.ID, "v1", "v2", []core.ThreadAnchor{next}))
	stored, err := f.threads.GetThread(t.Context(), moved)
	require.NoError(t, err)
	next.ThreadID = moved
	assert.Equal(t, &next, stored.Anchor)

	stale := core.ThreadAnchor{ThreadID: moved, StartLine: 1, EndLine: 1, Quote: "one"}
	require.NoError(t, f.threads.MoveAnchors(t.Context(), f.doc.ID, "v1", "v3", []core.ThreadAnchor{stale}))
	stored, err = f.threads.GetThread(t.Context(), moved)
	require.NoError(t, err)
	assert.Equal(t, 5, stored.Anchor.StartLine, "a thread already moved off v1 is skipped")

	empty, err := f.threads.AnchoredAt(t.Context(), f.doc.ID, "v1")
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestDeletingTheDocumentRemovesItsThreads(t *testing.T) {
	f := newThreadFixture(t)
	thread := f.newThread(t, nil, "first")
	_, err := f.docs.Delete(t.Context(), "shared", "plan.md")
	require.NoError(t, err)
	_, err = f.threads.GetThread(t.Context(), thread)
	require.ErrorIs(t, err, core.ErrNotFound)
}
