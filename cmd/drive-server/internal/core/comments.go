package core

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/pkg/randid"
)

// Comments is the thread service. A document is reached two ways: by its
// owner through a space and path, or by anyone through its share link. A
// link reaches only its own document's threads: thread and comment ids are
// not secrets, so every id a link call names is checked against the link's
// document.
type Comments struct {
	users    UserStores
	shares   *Shares
	accounts AccountStore
	notify   Notifier
	caps     CommentCaps
	now      func() time.Time
}

func NewComments(users UserStores, shares *Shares, accounts AccountStore, opts ...Option) *Comments {
	o := newOptions(opts)
	return &Comments{users: users, shares: shares, accounts: accounts, notify: o.notify, caps: o.caps, now: o.now}
}

// Target names a document: Space and Path for its owner, or Slug for a share
// link.
type Target struct {
	Space *drive.Space
	Path  string
	Slug  string
}

// scope is where an id-addressed call may reach: the caller's own file, or
// one owner's document through a link.
type scope struct {
	ownerID    string
	documentID string // empty for the caller's own file
}

func (s scope) allows(documentID string) bool {
	return s.documentID == "" || s.documentID == documentID
}

type access struct {
	ownerID string
	doc     Document
	body    []byte
}

func (c *Comments) resolve(ctx context.Context, p *Principal, t Target) (access, error) {
	if t.Slug != "" {
		link, _, err := c.shares.resolveLink(ctx, t.Slug)
		if err != nil {
			return access{}, err
		}
		store, release, err := c.users.Open(ctx, link.UserID)
		if err != nil {
			return access{}, fmt.Errorf("open user store: %w", err)
		}
		defer release()
		doc, body, err := store.GetByID(ctx, link.DocumentID)
		if IsNotFound(err) {
			return access{}, errLinkNotFound()
		}
		if err != nil {
			return access{}, fmt.Errorf("get shared document: %w", err)
		}
		return access{ownerID: link.UserID, doc: doc, body: body}, nil
	}

	if p == nil || t.Space == nil {
		return access{}, NewError(KindUnauthenticated, "not signed in")
	}
	if err := drive.ValidatePath(t.Path); err != nil {
		return access{}, invalidPath(err)
	}
	store, release, err := c.users.Open(ctx, p.UserID)
	if err != nil {
		return access{}, fmt.Errorf("open user store: %w", err)
	}
	defer release()
	doc, body, err := store.Get(ctx, t.Space.Key, t.Path)
	if err != nil {
		return access{}, notFoundOr(err, "get document")
	}
	return access{ownerID: p.UserID, doc: doc, body: body}, nil
}

// scopeFor is the reach of an id-addressed call: the caller's own file for
// an empty slug, else the link's one document.
func (c *Comments) scopeFor(ctx context.Context, p Principal, slug string) (scope, error) {
	if slug == "" {
		return scope{ownerID: p.UserID}, nil
	}
	link, _, err := c.shares.resolveLink(ctx, slug)
	if err != nil {
		return scope{}, err
	}
	return scope{ownerID: link.UserID, documentID: link.DocumentID}, nil
}

// Located is the document a stream follows, at its current version.
type Located struct {
	OwnerID    string
	DocumentID string
	SHA256     string
}

// Locate returns the document a stream for t follows. It is also how a link
// stream checks that its link still works.
func (c *Comments) Locate(ctx context.Context, p *Principal, t Target) (Located, error) {
	a, err := c.resolve(ctx, p, t)
	if err != nil {
		return Located{}, err
	}
	return Located{OwnerID: a.ownerID, DocumentID: a.doc.ID, SHA256: a.doc.SHA256}, nil
}

// List returns the document's threads. p is nil for an anonymous reader,
// who sees names but no account ids or emails.
func (c *Comments) List(ctx context.Context, p *Principal, t Target) (driveapi.ThreadList, error) {
	a, err := c.resolve(ctx, p, t)
	if err != nil {
		return driveapi.ThreadList{}, err
	}
	threads, release, err := c.users.OpenThreads(ctx, a.ownerID)
	if err != nil {
		return driveapi.ThreadList{}, fmt.Errorf("open thread store: %w", err)
	}
	defer release()
	stored, err := threads.ListThreads(ctx, a.doc.ID)
	if err != nil {
		return driveapi.ThreadList{}, fmt.Errorf("list threads: %w", err)
	}
	names, err := c.names(ctx, stored...)
	if err != nil {
		return driveapi.ThreadList{}, err
	}

	list := driveapi.ThreadList{DocumentSHA256: a.doc.SHA256, Threads: make([]driveapi.Thread, len(stored))}
	if p != nil {
		list.Viewer = driveapi.Viewer{UserID: p.UserID, IsOwner: p.UserID == a.ownerID, CanComment: p.Scope.Covers(ScopePush)}
	}
	for i, st := range stored {
		list.Threads[i] = names.thread(st, p == nil)
	}
	return list, nil
}

func (c *Comments) CreateThread(ctx context.Context, p Principal, t Target, in driveapi.CreateThread) (driveapi.Thread, error) {
	if err := canWrite(p); err != nil {
		return driveapi.Thread{}, err
	}
	body, err := checkBody(in.Body)
	if err != nil {
		return driveapi.Thread{}, err
	}
	if in.DocumentSHA256 == "" {
		return driveapi.Thread{}, NewError(KindInvalid, "document_sha256 is required")
	}
	a, err := c.resolve(ctx, &p, t)
	if err != nil {
		return driveapi.Thread{}, err
	}
	anchor, err := checkAnchor(a.doc.Kind, a.body, in.Anchor)
	if err != nil {
		return driveapi.Thread{}, err
	}

	threads, release, err := c.users.OpenThreads(ctx, a.ownerID)
	if err != nil {
		return driveapi.Thread{}, fmt.Errorf("open thread store: %w", err)
	}
	defer release()
	id := randid.UUIDv7()
	err = threads.InsertThread(ctx, NewThread{
		ID:         id,
		DocumentID: a.doc.ID,
		Anchor:     anchor,
		Author:     actorOf(p),
		CommentID:  randid.UUIDv7(),
		Body:       body,
		CreatedAt:  c.now(),
	}, in.DocumentSHA256, c.caps)
	if err != nil {
		return driveapi.Thread{}, notFoundOr(err, "create thread")
	}
	c.notify.ThreadsChanged(a.ownerID, a.doc.ID)
	return c.readThread(ctx, threads, id)
}

// Reply adds a comment to a thread. slug is the link the caller came
// through, or empty for the caller's own document.
func (c *Comments) Reply(ctx context.Context, p Principal, slug, threadID, body string) (driveapi.Comment, error) {
	if err := canWrite(p); err != nil {
		return driveapi.Comment{}, err
	}
	body, err := checkBody(body)
	if err != nil {
		return driveapi.Comment{}, err
	}
	sc, threads, release, err := c.open(ctx, p, slug)
	if err != nil {
		return driveapi.Comment{}, err
	}
	defer release()
	thread, err := c.thread(ctx, threads, sc, threadID)
	if err != nil {
		return driveapi.Comment{}, err
	}

	id := randid.UUIDv7()
	err = threads.InsertComment(ctx, NewComment{ID: id, ThreadID: thread.ID, Author: actorOf(p), Body: body, CreatedAt: c.now()}, c.caps)
	if IsNotFound(err) {
		return driveapi.Comment{}, errThreadNotFound()
	}
	if err != nil {
		return driveapi.Comment{}, fmt.Errorf("add comment: %w", err)
	}
	c.notify.ThreadsChanged(sc.ownerID, thread.DocumentID)
	return c.readComment(ctx, threads, id)
}

// SetResolved resolves or reopens a thread. The document's owner and the
// thread's author may.
func (c *Comments) SetResolved(ctx context.Context, p Principal, slug, threadID string, resolved bool) (driveapi.Thread, error) {
	if err := canWrite(p); err != nil {
		return driveapi.Thread{}, err
	}
	sc, threads, release, err := c.open(ctx, p, slug)
	if err != nil {
		return driveapi.Thread{}, err
	}
	defer release()
	thread, err := c.thread(ctx, threads, sc, threadID)
	if err != nil {
		return driveapi.Thread{}, err
	}
	if p.UserID != sc.ownerID && p.UserID != thread.Author.UserID {
		return driveapi.Thread{}, NewError(KindForbidden, "only the document's owner or the thread's author can resolve it")
	}

	var by *Actor
	if resolved {
		a := actorOf(p)
		by = &a
	}
	if err := threads.SetResolved(ctx, thread.ID, by, c.now()); err != nil {
		return driveapi.Thread{}, notFoundOr(err, "set resolved")
	}
	c.notify.ThreadsChanged(sc.ownerID, thread.DocumentID)
	return c.readThread(ctx, threads, thread.ID)
}

// EditComment changes a comment's body. Only its author may.
func (c *Comments) EditComment(ctx context.Context, p Principal, slug, commentID, body string) (driveapi.Comment, error) {
	if err := canWrite(p); err != nil {
		return driveapi.Comment{}, err
	}
	body, err := checkBody(body)
	if err != nil {
		return driveapi.Comment{}, err
	}
	sc, threads, release, err := c.open(ctx, p, slug)
	if err != nil {
		return driveapi.Comment{}, err
	}
	defer release()
	comment, thread, err := c.comment(ctx, threads, sc, commentID)
	if err != nil {
		return driveapi.Comment{}, err
	}
	if comment.Author.UserID != p.UserID {
		return driveapi.Comment{}, NewError(KindForbidden, "only a comment's author can edit it")
	}
	if len(body) > len(comment.Body) {
		if err := c.checkGrowth(ctx, threads, thread.DocumentID, len(body)-len(comment.Body)); err != nil {
			return driveapi.Comment{}, err
		}
	}
	if err := threads.UpdateComment(ctx, comment.ID, body, c.now()); err != nil {
		if IsNotFound(err) {
			return driveapi.Comment{}, errCommentNotFound()
		}
		return driveapi.Comment{}, fmt.Errorf("edit comment: %w", err)
	}
	c.notify.ThreadsChanged(sc.ownerID, thread.DocumentID)
	return c.readComment(ctx, threads, comment.ID)
}

// DeleteComment removes a comment. Its author and the document's owner may.
func (c *Comments) DeleteComment(ctx context.Context, p Principal, slug, commentID string) error {
	if err := canWrite(p); err != nil {
		return err
	}
	sc, threads, release, err := c.open(ctx, p, slug)
	if err != nil {
		return err
	}
	defer release()
	comment, thread, err := c.comment(ctx, threads, sc, commentID)
	if err != nil {
		return err
	}
	if comment.Author.UserID != p.UserID && p.UserID != sc.ownerID {
		return NewError(KindForbidden, "only a comment's author or the document's owner can delete it")
	}
	if _, err := threads.DeleteComment(ctx, comment.ID, c.now()); err != nil {
		if IsNotFound(err) {
			return errCommentNotFound()
		}
		return fmt.Errorf("delete comment: %w", err)
	}
	c.notify.ThreadsChanged(sc.ownerID, thread.DocumentID)
	return nil
}

func (c *Comments) open(ctx context.Context, p Principal, slug string) (scope, ThreadStore, func(), error) {
	sc, err := c.scopeFor(ctx, p, slug)
	if err != nil {
		return scope{}, nil, nil, err
	}
	threads, release, err := c.users.OpenThreads(ctx, sc.ownerID)
	if err != nil {
		return scope{}, nil, nil, fmt.Errorf("open thread store: %w", err)
	}
	return sc, threads, release, nil
}

func (c *Comments) thread(ctx context.Context, threads ThreadStore, sc scope, id string) (StoredThread, error) {
	t, err := threads.GetThread(ctx, id)
	if IsNotFound(err) || (err == nil && !sc.allows(t.DocumentID)) {
		return StoredThread{}, errThreadNotFound()
	}
	if err != nil {
		return StoredThread{}, fmt.Errorf("get thread: %w", err)
	}
	return t, nil
}

// comment returns a visible comment and its thread.
func (c *Comments) comment(ctx context.Context, threads ThreadStore, sc scope, id string) (StoredComment, StoredThread, error) {
	cm, err := threads.GetComment(ctx, id)
	if IsNotFound(err) || (err == nil && cm.DeletedAt != nil) {
		return StoredComment{}, StoredThread{}, errCommentNotFound()
	}
	if err != nil {
		return StoredComment{}, StoredThread{}, fmt.Errorf("get comment: %w", err)
	}
	t, err := c.thread(ctx, threads, sc, cm.ThreadID)
	if IsKind(err, KindNotFound) {
		return StoredComment{}, StoredThread{}, errCommentNotFound()
	}
	if err != nil {
		return StoredComment{}, StoredThread{}, err
	}
	return cm, t, nil
}

// checkGrowth holds an edit to the document's comment byte cap. Inserts
// check it in their own transaction; an edit is rare enough to check here.
func (c *Comments) checkGrowth(ctx context.Context, threads ThreadStore, documentID string, grow int) error {
	if c.caps.BodyBytesPerDocument <= 0 {
		return nil
	}
	all, err := threads.ListThreads(ctx, documentID)
	if err != nil {
		return fmt.Errorf("list threads: %w", err)
	}
	var used int64
	for _, t := range all {
		for _, cm := range t.Comments {
			used += int64(len(cm.Body))
		}
	}
	if used+int64(grow) > c.caps.BodyBytesPerDocument {
		return NewError(KindTooLarge, "this document has too much comment text")
	}
	return nil
}

func (c *Comments) readThread(ctx context.Context, threads ThreadStore, id string) (driveapi.Thread, error) {
	t, err := threads.GetThread(ctx, id)
	if err != nil {
		return driveapi.Thread{}, notFoundOr(err, "read back thread")
	}
	names, err := c.names(ctx, t)
	if err != nil {
		return driveapi.Thread{}, err
	}
	return names.thread(t, false), nil
}

func (c *Comments) readComment(ctx context.Context, threads ThreadStore, id string) (driveapi.Comment, error) {
	cm, err := threads.GetComment(ctx, id)
	if err != nil {
		return driveapi.Comment{}, notFoundOr(err, "read back comment")
	}
	names, err := c.names(ctx, StoredThread{Comments: []StoredComment{cm}})
	if err != nil {
		return driveapi.Comment{}, err
	}
	return names.comment(cm, false), nil
}

// authorNames maps account ids to the names shown for them.
type authorNames map[string]string

func (c *Comments) names(ctx context.Context, threads ...StoredThread) (authorNames, error) {
	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, t := range threads {
		add(t.Author.UserID)
		if t.ResolvedBy != nil {
			add(t.ResolvedBy.UserID)
		}
		for _, cm := range t.Comments {
			add(cm.Author.UserID)
		}
	}
	names := authorNames{}
	if len(ids) == 0 {
		return names, nil
	}
	accts, err := c.accounts.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("look up authors: %w", err)
	}
	for _, a := range accts {
		names[a.ID] = ShownName(a)
	}
	return names, nil
}

// author falls back to the stored email's local part for an account that is
// gone.
func (n authorNames) author(a Actor, anonymous bool) driveapi.Author {
	name, ok := n[a.UserID]
	if !ok {
		name = EmailName(a.Email)
	}
	out := driveapi.Author{Name: name, Agent: a.ViaToken}
	if !anonymous {
		out.ID, out.Email = a.UserID, a.Email
	}
	return out
}

func (n authorNames) thread(t StoredThread, anonymous bool) driveapi.Thread {
	out := driveapi.Thread{
		ID:         t.ID,
		Author:     n.author(t.Author, anonymous),
		ResolvedAt: t.ResolvedAt,
		CreatedAt:  t.CreatedAt,
		Comments:   make([]driveapi.Comment, len(t.Comments)),
	}
	if t.Anchor != nil {
		out.Anchor = &driveapi.Anchor{StartLine: t.Anchor.StartLine, EndLine: t.Anchor.EndLine, Quote: t.Anchor.Quote}
		out.Outdated = t.Anchor.Outdated
	}
	if t.ResolvedBy != nil {
		by := n.author(*t.ResolvedBy, anonymous)
		out.ResolvedBy = &by
	}
	for i, cm := range t.Comments {
		out.Comments[i] = n.comment(cm, anonymous)
	}
	return out
}

func (n authorNames) comment(c StoredComment, anonymous bool) driveapi.Comment {
	out := driveapi.Comment{ID: c.ID, ThreadID: c.ThreadID, CreatedAt: c.CreatedAt, EditedAt: c.EditedAt}
	if c.DeletedAt != nil {
		out.Deleted = true
		return out
	}
	out.Author = n.author(c.Author, anonymous)
	out.Body = c.Body
	return out
}

func canWrite(p Principal) error {
	if !p.Scope.Covers(ScopePush) {
		return NewError(KindForbidden, "token scope does not allow comments")
	}
	return nil
}

func checkBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	switch {
	case body == "":
		return "", NewError(KindInvalid, "comment is empty")
	case len(body) > MaxCommentBytes:
		return "", NewError(KindInvalid, fmt.Sprintf("comment exceeds %d bytes", MaxCommentBytes))
	case !utf8.ValidString(body):
		return "", NewError(KindInvalid, "comment is not valid UTF-8")
	}
	return body, nil
}

func checkAnchor(kind drive.Kind, body []byte, in *driveapi.Anchor) (*ThreadAnchor, error) {
	if in == nil {
		return nil, nil
	}
	if kind != drive.KindMarkdown {
		return nil, NewError(KindInvalid, "only markdown documents take anchored comments")
	}
	lines := lineCount(body)
	if in.StartLine < 1 || in.EndLine < in.StartLine || in.EndLine > lines {
		return nil, NewError(KindInvalid, fmt.Sprintf("anchor lines must be within 1..%d", lines))
	}
	quote := strings.TrimSpace(in.Quote)
	if n := utf8.RuneCountInString(quote); n == 0 || n > MaxQuoteRunes {
		return nil, NewError(KindInvalid, fmt.Sprintf("anchor quote must be 1..%d characters", MaxQuoteRunes))
	}
	return &ThreadAnchor{StartLine: in.StartLine, EndLine: in.EndLine, Quote: quote}, nil
}

// lineCount counts lines the way the renderer numbers them: a final line
// without a newline counts.
func lineCount(body []byte) int {
	n := bytes.Count(body, []byte("\n"))
	if len(body) > 0 && body[len(body)-1] != '\n' {
		n++
	}
	return n
}

func actorOf(p Principal) Actor {
	return Actor{UserID: p.UserID, Email: p.Email, ViaToken: p.TokenName}
}

func errThreadNotFound() error  { return NewError(KindNotFound, "thread not found") }
func errCommentNotFound() error { return NewError(KindNotFound, "comment not found") }
