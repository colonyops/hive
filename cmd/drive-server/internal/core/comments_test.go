package core_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/drive-server/internal/core"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
)

const sharedPlan = `---
title: Plan
---
# Plan

Keep the old ones for now.

Ship it on Friday.
`

type commentsEnv struct {
	*env
	owner core.Principal
	other core.Principal
	doc   driveapi.Document
	slug  string
}

func newCommentsEnv(t *testing.T) *commentsEnv {
	t.Helper()
	e := newEnv(t, devPolicy())
	owner := principalFor(t, e, "owner@example.com")
	other := principalFor(t, e, "bea@example.com")
	doc, err := e.docs.Put(t.Context(), owner, drive.Shared, "plan.md", []byte(sharedPlan))
	require.NoError(t, err)
	share, err := e.shares.Create(t.Context(), owner, drive.Shared, "plan.md", "")
	require.NoError(t, err)
	slug, _ := strings.CutPrefix(share.URL, testBaseURL+"/s/")
	e.notes.Take()
	return &commentsEnv{env: e, owner: owner, other: other, doc: doc, slug: slug}
}

func ownerTarget(path string) core.Target {
	space := drive.Shared
	return core.Target{Space: &space, Path: path}
}

func anchorAt(line int, quote string) *driveapi.Anchor {
	return &driveapi.Anchor{StartLine: line, EndLine: line, Quote: quote}
}

func (c *commentsEnv) thread(t *testing.T, p core.Principal, anchor *driveapi.Anchor, body string) driveapi.Thread {
	t.Helper()
	th, err := c.comments.CreateThread(t.Context(), p, core.Target{Slug: c.slug}, driveapi.CreateThread{DocumentSHA256: c.doc.SHA256, Anchor: anchor, Body: body})
	require.NoError(t, err)
	return th
}

func (c *commentsEnv) list(t *testing.T, p *core.Principal) driveapi.ThreadList {
	t.Helper()
	list, err := c.comments.List(t.Context(), p, core.Target{Slug: c.slug})
	require.NoError(t, err)
	return list
}

func readToken(t *testing.T, e *env, p core.Principal, scope core.Scope) core.Principal {
	t.Helper()
	secret, _, err := e.tokens.Mint(t.Context(), p.UserID, "claude", scope, nil)
	require.NoError(t, err)
	tp, err := e.tokens.Resolve(t.Context(), secret)
	require.NoError(t, err)
	return tp
}

func TestCreateThreadThroughLinkAndOwnerPath(t *testing.T) {
	c := newCommentsEnv(t)
	th := c.thread(t, c.other, anchorAt(6, "Keep the old ones"), "Why keep them?")

	assert.Equal(t, "bea", th.Author.Name)
	assert.Equal(t, c.other.UserID, th.Author.ID)
	require.Len(t, th.Comments, 1)
	assert.Equal(t, "Why keep them?", th.Comments[0].Body)
	assert.Equal(t, &driveapi.Anchor{StartLine: 6, EndLine: 6, Quote: "Keep the old ones"}, th.Anchor)
	assert.Equal(t, []string{"threads " + c.doc.ID}, c.notes.Take())

	_, err := c.comments.CreateThread(t.Context(), c.owner, ownerTarget("plan.md"), driveapi.CreateThread{DocumentSHA256: c.doc.SHA256, Body: "General note"})
	require.NoError(t, err)

	list, err := c.comments.List(t.Context(), &c.owner, ownerTarget("plan.md"))
	require.NoError(t, err)
	require.Len(t, list.Threads, 2)
	assert.Nil(t, list.Threads[0].Anchor, "general threads come first")
	assert.Equal(t, c.doc.SHA256, list.DocumentSHA256)
	assert.Equal(t, driveapi.Viewer{UserID: c.owner.UserID, IsOwner: true, CanComment: true}, list.Viewer)
}

func TestCreateThreadChecks(t *testing.T) {
	c := newCommentsEnv(t)
	create := func(in driveapi.CreateThread) error {
		_, err := c.comments.CreateThread(t.Context(), c.other, core.Target{Slug: c.slug}, in)
		return err
	}
	sha := c.doc.SHA256

	assert.True(t, core.IsKind(create(driveapi.CreateThread{DocumentSHA256: sha, Body: "  "}), core.KindInvalid), "empty body")
	assert.True(t, core.IsKind(create(driveapi.CreateThread{DocumentSHA256: sha, Body: strings.Repeat("x", core.MaxCommentBytes+1)}), core.KindInvalid), "long body")
	assert.True(t, core.IsKind(create(driveapi.CreateThread{Body: "x"}), core.KindInvalid), "no sha")
	assert.True(t, core.IsKind(create(driveapi.CreateThread{DocumentSHA256: sha, Body: "x", Anchor: anchorAt(9, "Ship")}), core.KindInvalid), "past the last line")
	assert.True(t, core.IsKind(create(driveapi.CreateThread{DocumentSHA256: sha, Body: "x", Anchor: &driveapi.Anchor{StartLine: 3, EndLine: 2, Quote: "x"}}), core.KindInvalid), "backwards range")
	assert.True(t, core.IsKind(create(driveapi.CreateThread{DocumentSHA256: sha, Body: "x", Anchor: anchorAt(6, " ")}), core.KindInvalid), "empty quote")

	err := create(driveapi.CreateThread{DocumentSHA256: "stale", Body: "x"})
	require.ErrorIs(t, err, core.ErrVersionChanged)
	assert.True(t, core.IsKind(err, core.KindConflict))
}

func TestCreateThreadOnHTMLTakesNoAnchor(t *testing.T) {
	c := newCommentsEnv(t)
	doc, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "page.html", []byte("<p>hi</p>\n"))
	require.NoError(t, err)

	_, err = c.comments.CreateThread(t.Context(), c.owner, ownerTarget("page.html"), driveapi.CreateThread{DocumentSHA256: doc.SHA256, Body: "x", Anchor: anchorAt(1, "hi")})
	assert.True(t, core.IsKind(err, core.KindInvalid))
	_, err = c.comments.CreateThread(t.Context(), c.owner, ownerTarget("page.html"), driveapi.CreateThread{DocumentSHA256: doc.SHA256, Body: "x"})
	assert.NoError(t, err)
}

func TestCaps(t *testing.T) {
	t.Run("threads per document", func(t *testing.T) {
		c := newCommentsEnv(t)
		for range testCommentCaps.ThreadsPerDocument {
			c.thread(t, c.other, nil, "x")
		}
		_, err := c.comments.CreateThread(t.Context(), c.other, core.Target{Slug: c.slug}, driveapi.CreateThread{DocumentSHA256: c.doc.SHA256, Body: "x"})
		assert.True(t, core.IsKind(err, core.KindTooLarge))
	})

	t.Run("comments per thread", func(t *testing.T) {
		c := newCommentsEnv(t)
		th := c.thread(t, c.other, nil, "x")
		for range testCommentCaps.CommentsPerThread - 1 {
			_, err := c.comments.Reply(t.Context(), c.other, c.slug, th.ID, "y")
			require.NoError(t, err)
		}
		_, err := c.comments.Reply(t.Context(), c.other, c.slug, th.ID, "y")
		assert.True(t, core.IsKind(err, core.KindTooLarge))
	})

	t.Run("bytes per document", func(t *testing.T) {
		c := newCommentsEnv(t)
		th := c.thread(t, c.other, nil, strings.Repeat("a", 150))
		_, err := c.comments.Reply(t.Context(), c.other, c.slug, th.ID, strings.Repeat("b", 60))
		assert.True(t, core.IsKind(err, core.KindTooLarge))

		reply, err := c.comments.Reply(t.Context(), c.other, c.slug, th.ID, strings.Repeat("b", 40))
		require.NoError(t, err)
		_, err = c.comments.EditComment(t.Context(), c.other, c.slug, reply.ID, strings.Repeat("b", 60))
		assert.True(t, core.IsKind(err, core.KindTooLarge), "an edit that grows past the cap")
	})
}

func TestPermissions(t *testing.T) {
	c := newCommentsEnv(t)
	third := principalFor(t, c.env, "cy@example.com")
	th := c.thread(t, c.other, nil, "first")
	reply, err := c.comments.Reply(t.Context(), third, c.slug, th.ID, "second")
	require.NoError(t, err)

	t.Run("resolve: owner and thread author only", func(t *testing.T) {
		_, err := c.comments.SetResolved(t.Context(), third, c.slug, th.ID, true)
		assert.True(t, core.IsKind(err, core.KindForbidden))

		got, err := c.comments.SetResolved(t.Context(), c.other, c.slug, th.ID, true)
		require.NoError(t, err)
		require.NotNil(t, got.ResolvedBy)
		assert.Equal(t, "bea", got.ResolvedBy.Name)

		got, err = c.comments.SetResolved(t.Context(), c.owner, "", th.ID, false)
		require.NoError(t, err)
		assert.Nil(t, got.ResolvedAt)
	})

	t.Run("edit: author only", func(t *testing.T) {
		_, err := c.comments.EditComment(t.Context(), c.owner, "", reply.ID, "changed")
		assert.True(t, core.IsKind(err, core.KindForbidden))
		got, err := c.comments.EditComment(t.Context(), third, c.slug, reply.ID, "changed")
		require.NoError(t, err)
		assert.Equal(t, "changed", got.Body)
		assert.NotNil(t, got.EditedAt)
	})

	t.Run("delete: author or owner", func(t *testing.T) {
		err := c.comments.DeleteComment(t.Context(), third, c.slug, th.Comments[0].ID)
		assert.True(t, core.IsKind(err, core.KindForbidden))
		require.NoError(t, c.comments.DeleteComment(t.Context(), c.owner, "", th.Comments[0].ID))

		list := c.list(t, &c.owner)
		require.Len(t, list.Threads, 1)
		first := list.Threads[0].Comments[0]
		assert.True(t, first.Deleted)
		assert.Empty(t, first.Body)
		assert.Empty(t, first.Author.Name)
	})

	t.Run("resolve rights stay with the thread author after the first comment goes", func(t *testing.T) {
		_, err := c.comments.SetResolved(t.Context(), c.other, c.slug, th.ID, true)
		require.NoError(t, err)
		_, err = c.comments.SetResolved(t.Context(), third, c.slug, th.ID, false)
		assert.True(t, core.IsKind(err, core.KindForbidden))
	})

	t.Run("deleting the last visible comment removes the thread", func(t *testing.T) {
		require.NoError(t, c.comments.DeleteComment(t.Context(), third, c.slug, reply.ID))
		assert.Empty(t, c.list(t, &c.owner).Threads)
	})
}

func TestLinkReachesOnlyItsDocument(t *testing.T) {
	c := newCommentsEnv(t)
	otherDoc, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "private.md", []byte("secret\n"))
	require.NoError(t, err)
	private, err := c.comments.CreateThread(t.Context(), c.owner, ownerTarget("private.md"), driveapi.CreateThread{DocumentSHA256: otherDoc.SHA256, Body: "private note"})
	require.NoError(t, err)

	_, err = c.comments.Reply(t.Context(), c.other, c.slug, private.ID, "got in")
	assert.True(t, core.IsKind(err, core.KindNotFound), "reply")
	_, err = c.comments.SetResolved(t.Context(), c.owner, c.slug, private.ID, true)
	assert.True(t, core.IsKind(err, core.KindNotFound), "resolve, even by the owner")
	_, err = c.comments.EditComment(t.Context(), c.owner, c.slug, private.Comments[0].ID, "x")
	assert.True(t, core.IsKind(err, core.KindNotFound), "edit")
	err = c.comments.DeleteComment(t.Context(), c.owner, c.slug, private.Comments[0].ID)
	assert.True(t, core.IsKind(err, core.KindNotFound), "delete")

	_, err = c.comments.Reply(t.Context(), c.other, "", private.ID, "own file")
	assert.True(t, core.IsKind(err, core.KindNotFound), "another user's own-file lookup")
}

func TestDeadLinkIsNotFound(t *testing.T) {
	c := newCommentsEnv(t)
	th := c.thread(t, c.other, nil, "x")
	c.notes.Take()
	require.NoError(t, c.shares.Revoke(t.Context(), c.owner, drive.Shared, "plan.md"))
	assert.Equal(t, []string{"link closed " + c.doc.ID}, c.notes.Take())

	_, err := c.comments.List(t.Context(), nil, core.Target{Slug: c.slug})
	assert.True(t, core.IsKind(err, core.KindNotFound))
	_, err = c.comments.Reply(t.Context(), c.other, c.slug, th.ID, "x")
	assert.True(t, core.IsKind(err, core.KindNotFound))
	_, _, err = c.comments.Locate(t.Context(), nil, core.Target{Slug: c.slug})
	assert.True(t, core.IsKind(err, core.KindNotFound))
}

func TestAnonymousListHidesIdentity(t *testing.T) {
	c := newCommentsEnv(t)
	th := c.thread(t, c.other, nil, "x")
	_, err := c.comments.SetResolved(t.Context(), c.other, c.slug, th.ID, true)
	require.NoError(t, err)

	list := c.list(t, nil)
	assert.Equal(t, driveapi.Viewer{}, list.Viewer)
	got := list.Threads[0]
	assert.Equal(t, driveapi.Author{Name: "bea"}, got.Author)
	assert.Equal(t, driveapi.Author{Name: "bea"}, got.Comments[0].Author)
	assert.Equal(t, &driveapi.Author{Name: "bea"}, got.ResolvedBy)

	signedIn := c.list(t, &c.other)
	assert.Equal(t, "bea@example.com", signedIn.Threads[0].Author.Email)
	assert.False(t, signedIn.Viewer.IsOwner)
	assert.True(t, signedIn.Viewer.CanComment)
}

func TestReadTokenCannotComment(t *testing.T) {
	c := newCommentsEnv(t)
	reader := readToken(t, c.env, c.owner, core.ScopeRead)

	list, err := c.comments.List(t.Context(), &reader, ownerTarget("plan.md"))
	require.NoError(t, err)
	assert.False(t, list.Viewer.CanComment)

	_, err = c.comments.CreateThread(t.Context(), reader, ownerTarget("plan.md"), driveapi.CreateThread{DocumentSHA256: c.doc.SHA256, Body: "x"})
	assert.True(t, core.IsKind(err, core.KindForbidden))
}

func TestTokenCommentsShowTheAgent(t *testing.T) {
	c := newCommentsEnv(t)
	agent := readToken(t, c.env, c.owner, core.ScopePush)
	th := c.thread(t, c.other, nil, "question")

	reply, err := c.comments.Reply(t.Context(), agent, "", th.ID, "answer")
	require.NoError(t, err)
	assert.Equal(t, "claude", reply.Author.Agent)
	assert.Equal(t, "owner", reply.Author.Name)
}

func TestAuthorNames(t *testing.T) {
	c := newCommentsEnv(t)
	th := c.thread(t, c.other, nil, "x")

	_, err := c.accounts.SetDisplayName(t.Context(), c.other, "  Bea \n Smith ")
	require.NoError(t, err)
	assert.Equal(t, "Bea Smith", c.list(t, nil).Threads[0].Author.Name)

	require.NoError(t, c.accounts.Delete(t.Context(), c.other.Email))
	assert.Equal(t, "bea", c.list(t, nil).Threads[0].Author.Name, "a deleted account falls back to its stored email")
	assert.Equal(t, th.ID, c.list(t, nil).Threads[0].ID)
}

func TestSetDisplayNameCaps(t *testing.T) {
	c := newCommentsEnv(t)
	acct, err := c.accounts.SetDisplayName(t.Context(), c.owner, strings.Repeat("n", 100))
	require.NoError(t, err)
	assert.Len(t, acct.DisplayName, core.MaxDisplayName)

	acct, err = c.accounts.SetDisplayName(t.Context(), c.owner, "")
	require.NoError(t, err)
	assert.Empty(t, acct.DisplayName)
}

const sharedPlanV2 = `---
title: Plan
---
# Plan

Intro line.

Keep the old ones for now, maybe.

Ship it on Friday.
`

func TestPutReanchorsThreads(t *testing.T) {
	c := newCommentsEnv(t)
	moved := c.thread(t, c.other, anchorAt(8, "Ship it on Friday"), "moves down")
	edited := c.thread(t, c.other, anchorAt(6, "Keep the old ones"), "line edited")
	general := c.thread(t, c.other, nil, "general")
	c.notes.Take()

	_, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(sharedPlanV2))
	require.NoError(t, err)
	assert.Equal(t, []string{"threads " + c.doc.ID, "document " + c.doc.ID}, c.notes.Take())

	byID := func(list driveapi.ThreadList) map[string]driveapi.Thread {
		m := map[string]driveapi.Thread{}
		for _, th := range list.Threads {
			m[th.ID] = th
		}
		return m
	}
	got := byID(c.list(t, &c.owner))
	assert.Equal(t, 10, got[moved.ID].Anchor.StartLine)
	assert.Equal(t, 8, got[edited.ID].Anchor.StartLine)
	assert.False(t, got[edited.ID].Outdated)
	assert.Nil(t, got[general.ID].Anchor)

	v3 := strings.Replace(sharedPlanV2, "Ship it on Friday.\n", "", 1)
	_, err = c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(v3))
	require.NoError(t, err)
	got = byID(c.list(t, &c.owner))
	assert.True(t, got[moved.ID].Outdated)
	assert.Equal(t, "Ship it on Friday", got[moved.ID].Anchor.Quote)

	_, err = c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(sharedPlanV2))
	require.NoError(t, err)
	got = byID(c.list(t, &c.owner))
	assert.False(t, got[moved.ID].Outdated, "the quote came back")
	assert.Equal(t, 10, got[moved.ID].Anchor.StartLine)
}

func TestPutWithSameBodySendsNothing(t *testing.T) {
	c := newCommentsEnv(t)
	_, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(sharedPlan))
	require.NoError(t, err)
	assert.Empty(t, c.notes.Take())
}

func TestPutReanchorsAThreadCreatedDuringThePush(t *testing.T) {
	c := newCommentsEnv(t)
	var raced driveapi.Thread
	core.SetBeforePut(c.docs, func() {
		raced = c.thread(t, c.other, anchorAt(8, "Ship it on Friday"), "raced")
	})
	_, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(sharedPlanV2))
	require.NoError(t, err)
	core.SetBeforePut(c.docs, nil)

	list := c.list(t, &c.owner)
	require.Len(t, list.Threads, 1)
	assert.Equal(t, raced.ID, list.Threads[0].ID)
	assert.Equal(t, 10, list.Threads[0].Anchor.StartLine)
}

func TestPutReanchorsAThreadAnEarlierPushMissed(t *testing.T) {
	c := newCommentsEnv(t)
	missed := c.thread(t, c.other, anchorAt(8, "Ship it on Friday"), "missed")
	threads, release, err := c.users.OpenThreads(t.Context(), c.owner.UserID)
	require.NoError(t, err)
	stranded := core.ThreadAnchor{ThreadID: missed.ID, StartLine: 2, EndLine: 2, Quote: "Ship it on Friday"}
	require.NoError(t, threads.MoveAnchors(t.Context(), c.doc.ID, c.doc.SHA256, "a version that is gone", []core.ThreadAnchor{stranded}))
	release()

	_, err = c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(sharedPlanV2))
	require.NoError(t, err)

	list := c.list(t, &c.owner)
	require.Len(t, list.Threads, 1)
	assert.False(t, list.Threads[0].Outdated)
	assert.Equal(t, 10, list.Threads[0].Anchor.StartLine, "found by its quote, not its stale lines")
}

func TestPushesToOnePathRunInOrder(t *testing.T) {
	c := newCommentsEnv(t)
	moved := c.thread(t, c.other, anchorAt(8, "Ship it on Friday"), "moves")
	v3 := "Intro.\n\n" + sharedPlanV2

	var once sync.Once
	second := make(chan error, 1)
	core.SetBeforePut(c.docs, func() {
		once.Do(func() {
			go func() {
				_, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(v3))
				second <- err
			}()
			// Without the lock the second push would finish here, and the
			// first would then overwrite v3 with v2.
			select {
			case err := <-second:
				second <- err
			case <-time.After(100 * time.Millisecond):
			}
		})
	})
	_, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(sharedPlanV2))
	require.NoError(t, err)
	require.NoError(t, <-second)
	core.SetBeforePut(c.docs, nil)

	_, body, err := c.docs.Get(t.Context(), c.owner, drive.Shared, "plan.md")
	require.NoError(t, err)
	assert.Equal(t, v3, string(body))
	list := c.list(t, &c.owner)
	require.Len(t, list.Threads, 1)
	assert.Equal(t, moved.ID, list.Threads[0].ID)
	assert.Equal(t, 12, list.Threads[0].Anchor.StartLine)
}

func TestDeleteClosesTheDocument(t *testing.T) {
	c := newCommentsEnv(t)
	c.thread(t, c.other, nil, "x")
	c.notes.Take()
	require.NoError(t, c.docs.Delete(t.Context(), c.owner, drive.Shared, "plan.md"))
	assert.Equal(t, []string{"closed " + c.doc.ID}, c.notes.Take())

	_, err := c.docs.Put(t.Context(), c.owner, drive.Shared, "plan.md", []byte(sharedPlan))
	require.NoError(t, err)
	list, err := c.comments.List(t.Context(), &c.owner, ownerTarget("plan.md"))
	require.NoError(t, err)
	assert.Empty(t, list.Threads, "a delete takes the threads with it")
}

func TestCommentTimesUseTheClock(t *testing.T) {
	c := newCommentsEnv(t)
	th := c.thread(t, c.other, nil, "x")
	assert.True(t, th.CreatedAt.Equal(c.clock.Now()))
	c.clock.Advance(time.Minute)
	reply, err := c.comments.Reply(t.Context(), c.other, c.slug, th.ID, "y")
	require.NoError(t, err)
	assert.True(t, reply.CreatedAt.Equal(c.clock.Now()))
}
