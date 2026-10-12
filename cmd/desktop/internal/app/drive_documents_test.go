package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDocumentRefusesFrontMatterThatDoesNotParse(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.WriteDocument(ctx, "1", "plans/a.md", []byte("---\ntitle: [unclosed\n---\nbody\n"))
	assert.Equal(t, KindInvalid, KindOf(err))

	long := "---\nnotes: " + strings.Repeat("x", frontmatterIndexBytes) + "\n---\nbody\n"
	_, err = f.svc.WriteDocument(ctx, "1", "plans/a.md", []byte(long))
	assert.Equal(t, KindInvalid, KindOf(err), "front matter the index cannot reach would be silently untitled")
	f.signals.requireNoUpdates(t, "a refused write never notifies")

	entry, err := f.svc.WriteDocument(ctx, "1", "plans/a.md", []byte("---\ntitle: A\n---\nbody\n"))
	require.NoError(t, err)
	assert.Equal(t, "A", entry.Title)
	_, err = f.svc.WriteDocument(ctx, "1", "page.html", []byte("---\ntitle: [x\n---\n"))
	require.NoError(t, err, "html has no front matter to parse")
}

func TestWriteDocumentCapsTheBody(t *testing.T) {
	f := newDriveFixture(t)

	_, err := f.svc.WriteDocument(t.Context(), "1", "big.md", make([]byte, maxDocumentBytes+1))
	assert.Equal(t, KindInvalid, KindOf(err))
	_, err = f.svc.WriteDocument(t.Context(), "1", "big.md", []byte(strings.Repeat("x", maxDocumentBytes)))
	require.NoError(t, err)
}

func TestReadDocumentRefusesCanvasesAndFolders(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.PutBlock(ctx, "1", "reports/perf", "", "", markdownBlock("a", "x"))
	require.NoError(t, err)
	_, _, err = f.svc.ReadDocument(ctx, "1", "reports/perf.canvas.json")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, err.Error(), "read_canvas")

	_, _, err = f.svc.ReadDocument(ctx, "1", "reports")
	assert.Equal(t, KindInvalid, KindOf(err))
	_, _, err = f.svc.ReadDocument(ctx, "1", "ghost.md")
	assert.Equal(t, KindNotFound, KindOf(err))
}

func TestMoveDocumentKeepsTheKind(t *testing.T) {
	f := newDriveFixture(t)
	ctx := t.Context()

	_, err := f.svc.WriteDocument(ctx, "1", "plan.md", []byte("x"))
	require.NoError(t, err)
	_, err = f.svc.MoveDocument(ctx, "1", "plan.md", "plan.html")
	assert.Equal(t, KindInvalid, KindOf(err))
	assert.Contains(t, errorMessage(t, err), "keep the extension")

	entry, err := f.svc.MoveDocument(ctx, "1", "plan.md", "plans/plan.md")
	require.NoError(t, err)
	assert.Equal(t, "plans/plan.md", entry.Path)

	updates := f.signals.waitUpdates(t, 2)
	assert.Equal(t, []string{"plan.md"}, updates[0].Paths)
	assert.Equal(t, []string{"plan.md", "plans/plan.md"}, updates[1].Paths, "a move names both ends")
	assert.Equal(t, "1", updates[1].Session)
}

// errorMessage is what the UI shows: the message alone, without the cause.
func errorMessage(t *testing.T, err error) string {
	t.Helper()
	appErr, ok := errors.AsType[*Error](err)
	require.True(t, ok, "%v is not an app error", err)
	return appErr.Msg
}
