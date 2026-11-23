package wire

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/domain/review"
)

func TestDiffReviewsCrossTheWire(t *testing.T) {
	in := []review.DiffReview{{
		ID: "r1", SessionID: "s1", Author: "claude", Agent: "claude", Head: "abc", Summary: "one bug",
		CreatedAt: time.Date(2026, 11, 22, 10, 0, 0, 0, time.Local),
		Comments: []review.DiffReviewComment{
			{
				ID: "c1", ReviewID: "r1", Path: "a.go", StartSide: review.DiffSideDeletions, StartLine: 3,
				EndSide: review.DiffSideAdditions, EndLine: 5, Quote: "-x\n+y", Text: "why", Severity: review.SeverityIssue,
			},
			{ID: "c2", ReviewID: "r1", Kind: review.DiffCommentFile, Path: "b.go", Text: "test it", Severity: review.SeverityNote},
		},
	}}
	assert.Equal(t, in, DiffReviewsFromProto(DiffReviewsToProto(in)))
	assert.Empty(t, DiffReviewsFromProto(DiffReviewsToProto(nil)))
}
