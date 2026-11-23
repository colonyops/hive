//go:build remote

package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/review"
)

// An agent on a server reviews its branch with the hive CLI there. Desktop
// reads the review and dismisses its comments through the server's daemon,
// and the server's own CLI sees the dismissal.
func TestServerDiffReviews_ListAndDismissOverTheDaemon(t *testing.T) {
	created := serverA.CreateSession(t, "diff-review")
	serverA.Shell(t, "cd "+shellQuote(created.Path)+" && git checkout -qb review-me && printf 'one\ntwo\n' > reviewed.txt && git add reviewed.txt && "+
		"git -c user.name=t -c user.email=t@t -c commit.gpgsign=false commit -qm 'add reviewed.txt'").Ok(t)
	put := `{"author":"claude","agent":"claude","summary":"two notes","comments":[` +
		`{"path":"reviewed.txt","startLine":2,"severity":"issue","text":"line two is wrong"},` +
		`{"path":"reviewed.txt","kind":"file","text":"add a test"}]}`
	serverA.Shell(t, "echo "+shellQuote(put)+" | hive review diff put --session-id "+shellQuote(created.ID)).Ok(t)

	api := serverA.API(t)
	reviews, err := api.DiffReviews(t.Context(), created.ID)
	require.NoError(t, err)
	require.Len(t, reviews, 1)
	got := reviews[0]
	assert.Equal(t, "claude", got.Author)
	assert.Equal(t, "two notes", got.Summary)
	require.Len(t, got.Comments, 2)
	assert.Equal(t, "+two", got.Comments[0].Quote, "the server quotes the rows from its own checkout")
	assert.Equal(t, review.DiffCommentFile, got.Comments[1].Kind)

	require.NoError(t, api.DismissDiffComment(t.Context(), got.Comments[0].ID))
	require.ErrorIs(t, api.DismissDiffComment(t.Context(), got.Comments[0].ID), review.ErrDiffReviewNotFound)
	shown := serverA.Hive(t, "review", "diff", "show", "--session-id", created.ID).Ok(t)
	assert.NotContains(t, shown.Stdout, "line two is wrong")
	assert.Contains(t, shown.Stdout, "add a test")

	require.NoError(t, api.DismissDiffReview(t.Context(), got.ID))
	reviews, err = api.DiffReviews(t.Context(), created.ID)
	require.NoError(t, err)
	assert.Empty(t, reviews)
}
