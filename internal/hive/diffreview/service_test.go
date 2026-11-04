package diffreview

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/review"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/gitstatus"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/internal/store"
	"github.com/colonyops/hive/internal/store/db"
)

const servicePatch = `@@ -1,3 +1,4 @@
 package a
-var x = 1
+var x = 2
+var y = 3
 func f() {}
`

type fakeChanges struct {
	base  gitstatus.DiffBase
	files []git.ChangedFile
	diffs map[string]gitstatus.FileDiff
}

func (f fakeChanges) ReadChanges(context.Context, session.Session, gitstatus.DiffTarget) (gitstatus.Changes, error) {
	return gitstatus.Changes{Base: f.base, DefaultBranch: "main", Files: f.files}, nil
}

func (f fakeChanges) ReadFileDiffs(_ context.Context, _ session.Session, _ gitstatus.DiffTarget, paths []string) ([]gitstatus.FileDiff, error) {
	out := make([]gitstatus.FileDiff, 0, len(paths))
	for _, p := range paths {
		d, ok := f.diffs[p]
		if !ok {
			d = gitstatus.FileDiff{Path: p, State: gitstatus.FileDiffStateGone}
		}
		out = append(out, d)
	}
	return out, nil
}

type fakeCommits struct{}

func (fakeCommits) ResolveCommit(context.Context, string, string) (string, error) {
	return "abc123", nil
}

type sessionsFromStore struct{ store *store.SessionStore }

func (s sessionsFromStore) GetSession(ctx context.Context, id string) (session.Session, error) {
	return s.store.Get(ctx, id)
}

func newService(t *testing.T, changes fakeChanges) *Service {
	t.Helper()
	database := db.OpenForTest(t)
	sessions := store.NewSessionStore(database)
	now := time.Now()
	require.NoError(t, sessions.Save(t.Context(), session.Session{
		ID: "s1", Name: "feature", Slug: "feature", Path: "/tmp/feature", State: session.StateActive, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, sessions.Save(t.Context(), session.Session{
		ID: "s2", Name: "old", Slug: "old", Path: "/tmp/old", State: session.StateRecycled, CreatedAt: now, UpdatedAt: now,
	}))
	return NewService(zerolog.Nop(), store.NewDiffReviewStore(database), sessionsFromStore{sessions}, changes, fakeCommits{})
}

func branchChanges() fakeChanges {
	return fakeChanges{
		base:  gitstatus.DiffBaseBranch,
		files: []git.ChangedFile{{Path: "a.go"}, {Path: "img.png"}},
		diffs: map[string]gitstatus.FileDiff{
			"a.go":    {Path: "a.go", State: gitstatus.FileDiffStateText, Text: servicePatch},
			"img.png": {Path: "img.png", State: gitstatus.FileDiffStateBinary},
		},
	}
}

func TestPutQuotesRowsAndFillsDefaults(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, branchChanges())

	got, err := svc.Put(ctx, PutInput{
		SessionID: "s1", Author: " claude ", Agent: "Claude", Summary: "looks fine",
		Comments: []CommentInput{
			{Path: "./a.go", StartLine: 2, EndLine: 3, Text: "two lines", Severity: review.SeverityIssue},
			{Path: "a.go", StartSide: review.DiffSideDeletions, StartLine: 2, Text: "why remove"},
			{Path: "img.png", Kind: review.DiffCommentFile, StartLine: 9, Text: "binary"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "claude", got.Author)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, "abc123", got.Head)
	require.Len(t, got.Comments, 3)
	assert.Equal(t, "+var x = 2\n+var y = 3", got.Comments[0].Quote)
	assert.Equal(t, review.SeverityIssue, got.Comments[0].Severity)
	assert.Equal(t, "-var x = 1", got.Comments[1].Quote)
	assert.Equal(t, review.DiffSideDeletions, got.Comments[1].EndSide)
	assert.Equal(t, review.SeveritySuggestion, got.Comments[1].Severity)
	assert.Zero(t, got.Comments[2].StartLine, "a file comment has no lines")

	listed, err := svc.List(ctx, "s1")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, got.ID, listed[0].ID)
}

func TestPutReportsEveryProblem(t *testing.T) {
	svc := newService(t, branchChanges())
	_, err := svc.Put(context.Background(), PutInput{
		SessionID: "s1", Author: "claude",
		Comments: []CommentInput{
			{Path: "missing.go", StartLine: 1, Text: "x"},
			{Path: "a.go", StartLine: 1, Text: ""},
			{Path: "a.go", StartLine: 1, Text: "x", Severity: "blocker"},
		},
	})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "comment 1 (missing.go): the file is not in the branch diff against main")
	assert.Contains(t, err.Error(), "comment 2 (a.go): a comment needs text")
	assert.Contains(t, err.Error(), `comment 3 (a.go): severity "blocker"`)
}

func TestPutRefusesLinesOutsideTheDiff(t *testing.T) {
	svc := newService(t, branchChanges())
	_, err := svc.Put(context.Background(), PutInput{
		SessionID: "s1", Author: "claude",
		Comments: []CommentInput{
			{Path: "a.go", StartLine: 40, Text: "far away"},
			{Path: "img.png", StartLine: 1, Text: "lines in a binary"},
		},
	})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "comment 1 (a.go): L40 is not a run of rows in the branch diff")
	assert.Contains(t, err.Error(), "comment 2 (img.png): git shows no text diff")
}

func TestPutNeedsABranchAndALiveSession(t *testing.T) {
	ctx := context.Background()
	noBranch := branchChanges()
	noBranch.base = gitstatus.DiffBaseUncommitted
	_, err := newService(t, noBranch).Put(ctx, PutInput{SessionID: "s1", Author: "claude", Summary: "x"})
	require.ErrorIs(t, err, ErrInvalid)
	assert.Contains(t, err.Error(), "no branch to compare against main")

	svc := newService(t, branchChanges())
	_, err = svc.Put(ctx, PutInput{SessionID: "s2", Author: "claude", Summary: "x"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = svc.Put(ctx, PutInput{SessionID: "nope", Author: "claude", Summary: "x"})
	require.ErrorIs(t, err, session.ErrNotFound)
	_, err = svc.Put(ctx, PutInput{SessionID: "s1", Author: "claude"})
	require.ErrorIs(t, err, ErrInvalid, "an empty review is refused")
}

func TestClearAndDismiss(t *testing.T) {
	ctx := context.Background()
	svc := newService(t, branchChanges())
	first, err := svc.Put(ctx, PutInput{SessionID: "s1", Author: "claude", Comments: []CommentInput{{Path: "a.go", StartLine: 2, Text: "x"}}})
	require.NoError(t, err)
	_, err = svc.Put(ctx, PutInput{SessionID: "s1", Author: "codex", Summary: "fine"})
	require.NoError(t, err)
	_, err = svc.Put(ctx, PutInput{SessionID: "s1", Author: "pi", Summary: "fine"})
	require.NoError(t, err)

	require.NoError(t, svc.Dismiss(ctx, first.Comments[0].ID))

	n, err := svc.Clear(ctx, "s1", "claude")
	require.NoError(t, err)
	assert.Zero(t, n, "the review went with its last comment")

	n, err = svc.Clear(ctx, "s1", "codex")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	_, err = svc.Clear(ctx, "nope", "pi")
	require.ErrorIs(t, err, session.ErrNotFound)

	n, err = svc.Clear(ctx, "s1", "")
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}
