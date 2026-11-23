package drive

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/ops"
)

var chatCaller = Caller{Space: ws, Author: drive.Author{Session: "1"}}

func requireOpError(t *testing.T, err error, kind ops.Kind) *ops.Error {
	t.Helper()
	opErr, ok := errors.AsType[*ops.Error](err)
	require.True(t, ok, "%v is not an *ops.Error", err)
	assert.Equal(t, kind, opErr.Kind, opErr.Msg)
	return opErr
}

func TestPutDocumentCapsTheBody(t *testing.T) {
	f := newFixture(t, stubGit{})

	_, err := f.svc.PutDocument(t.Context(), chatCaller, "big.md", make([]byte, MaxDocumentBytes+1))
	assert.Equal(t, "invalid: document is too large (2097152 bytes max)", err.Error())
	_, err = f.svc.PutDocument(t.Context(), chatCaller, "big.md", []byte(strings.Repeat("x", MaxDocumentBytes)))
	require.NoError(t, err)
}

func TestPutDocumentRefusals(t *testing.T) {
	f := newFixture(t, stubGit{})
	ctx := t.Context()

	_, err := f.svc.PutDocument(ctx, chatCaller, "notes.txt", []byte("x"))
	requireOpError(t, err, ops.KindInvalid)
	_, err = f.svc.PutDocument(ctx, chatCaller, "a.md", []byte{0xff})
	requireOpError(t, err, ops.KindInvalid)
	_, err = f.svc.PutDocument(ctx, chatCaller, "../a.md", []byte("x"))
	requireOpError(t, err, ops.KindInvalid)

	_, err = f.svc.PutDocument(ctx, chatCaller, "plans/a.md", []byte("---\ntitle: [unclosed\n---\nbody\n"))
	assert.Contains(t, requireOpError(t, err, ops.KindInvalid).Msg, "front matter does not parse: ", "the parser's reason crosses")

	long := "---\nnotes: " + strings.Repeat("x", frontmatterHeadBytes) + "\n---\nbody\n"
	_, err = f.svc.PutDocument(ctx, chatCaller, "plans/a.md", []byte(long))
	assert.Contains(t, requireOpError(t, err, ops.KindInvalid).Msg, "front matter must close within the first 8192 bytes",
		"front matter the index cannot reach would be silently untitled")
	assert.Empty(t, f.changes(), "a refused write never publishes")

	entry, err := f.svc.PutDocument(ctx, chatCaller, "plans/a.md", []byte("---\ntitle: A\n---\nbody\n"))
	require.NoError(t, err)
	assert.Equal(t, "A", entry.Title)
	_, err = f.svc.PutDocument(ctx, chatCaller, "page.html", []byte("---\ntitle: [x\n---\n"))
	require.NoError(t, err, "html has no front matter to parse")
}

func TestGetDocument(t *testing.T) {
	f := newFixture(t, stubGit{})
	ctx := t.Context()
	f.write(t, ws, "notes.txt", "plain")
	f.write(t, ws, "kv.json", "{}")
	f.write(t, ws, "plans/kv.json.lock", "")
	f.write(t, ws, "bin.dat", "\xff\xfe")
	f.write(t, ws, "reports/perf.md", "x")

	_, body, err := f.svc.GetDocument(ctx, chatCaller, "notes.txt")
	require.NoError(t, err)
	assert.Equal(t, "plain", string(body))

	for _, path := range []string{"kv.json", "plans/kv.json.lock", "ghost.md"} {
		_, _, err = f.svc.GetDocument(ctx, chatCaller, path)
		requireOpError(t, err, ops.KindNotFound)
	}
	_, _, err = f.svc.GetDocument(ctx, chatCaller, "reports")
	requireOpError(t, err, ops.KindInvalid)
	_, _, err = f.svc.GetDocument(ctx, chatCaller, "bin.dat")
	requireOpError(t, err, ops.KindInvalid)
}

func TestListDocuments(t *testing.T) {
	f := newFixture(t, stubGit{})
	ctx := t.Context()
	f.write(t, ws, "plans/a.md", "x")
	f.write(t, ws, "plans/deep/b.md", "x")

	root, err := f.svc.ListDocuments(ctx, chatCaller, "", false)
	require.NoError(t, err)
	require.Len(t, root.Children, 1)
	assert.Empty(t, root.Children[0].Children, "children lists one level")

	tree, err := f.svc.ListDocuments(ctx, chatCaller, "/plans/", true)
	require.NoError(t, err)
	assert.Equal(t, []string{"plans/deep", "plans/a.md"}, paths(tree.Children))
	assert.Equal(t, []string{"plans/deep/b.md"}, paths(tree.Children[0].Children))

	doc, err := f.svc.ListDocuments(ctx, chatCaller, "plans/a.md", true)
	require.NoError(t, err)
	assert.Equal(t, "plans/a.md", doc.Path, "a document path lists that document alone")

	_, err = f.svc.ListDocuments(ctx, chatCaller, "ghost", false)
	assert.Contains(t, requireOpError(t, err, ops.KindNotFound).Msg, "an empty path lists the space root")
}

func TestMoveDocument(t *testing.T) {
	f := newFixture(t, stubGit{})
	ctx := t.Context()
	for _, path := range []string{"a.md", "b.md"} {
		_, err := f.svc.PutDocument(ctx, chatCaller, path, []byte("x"))
		require.NoError(t, err)
	}

	_, err := f.svc.MoveDocument(ctx, chatCaller, "a.md", "a.html")
	assert.Contains(t, requireOpError(t, err, ops.KindInvalid).Msg, "keep the extension")
	_, err = f.svc.MoveDocument(ctx, chatCaller, "a.md", "b.md")
	assert.Contains(t, requireOpError(t, err, ops.KindConflict).Msg, `"b.md" already exists`, "a conflict names the target")
	_, err = f.svc.MoveDocument(ctx, chatCaller, "ghost.md", "c.md")
	assert.Contains(t, requireOpError(t, err, ops.KindNotFound).Msg, `"ghost.md"`, "a missing source names the source")

	entry, err := f.svc.MoveDocument(ctx, chatCaller, "a.md", "plans/a.md")
	require.NoError(t, err)
	assert.Equal(t, "plans/a.md", entry.Path)
}

func TestDeleteDocument(t *testing.T) {
	f := newFixture(t, stubGit{})
	ctx := t.Context()
	_, err := f.svc.PutDocument(ctx, chatCaller, "plans/a.md", []byte("x"))
	require.NoError(t, err)

	err = f.svc.DeleteDocument(ctx, chatCaller, "plans")
	assert.Contains(t, requireOpError(t, err, ops.KindInvalid).Msg, "is not empty")
	require.NoError(t, f.svc.DeleteDocument(ctx, chatCaller, "plans/a.md"))
	requireOpError(t, f.svc.DeleteDocument(ctx, chatCaller, "plans/a.md"), ops.KindNotFound)
}

func TestCallersAndTheirSpaces(t *testing.T) {
	f := newFixture(t, stubGit{})

	c, err := f.svc.SessionCaller(session.Session{ID: "abc", Name: "fix", Remote: "git@github.com:acme/site.git"})
	require.NoError(t, err)
	assert.Equal(t, acme, c.Space)
	assert.Equal(t, drive.Author{HiveSession: "abc"}, c.Author)
	require.NotNil(t, c.Session)

	_, err = f.svc.SessionCaller(session.Session{ID: "zzz", Name: "scratch"})
	requireOpError(t, err, ops.KindInvalid)

	c, err = ChatCaller(Chat{ID: "1", Workspace: "ws"})
	require.NoError(t, err)
	assert.Equal(t, ws, c.Space)
	assert.Equal(t, drive.Author{Session: "1"}, c.Author)

	_, err = ChatCaller(Chat{ID: "1", Workspace: "../x"})
	requireOpError(t, err, ops.KindInvalid)
}
