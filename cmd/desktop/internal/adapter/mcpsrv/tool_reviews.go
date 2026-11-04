package mcpsrv

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/colonyops/hive/internal/ops/mcpops"
	"github.com/colonyops/hive/internal/ops/reviews"
)

func (ctrl *DriveController) registerReviews(srv *mcp.Server) {
	mcpops.Mount(srv, reviews.DiffReviewPut, ctrl.PutDiffReview, ctrl.toolError)
	mcpops.Mount(srv, reviews.DiffReviewGet, ctrl.GetDiffReviews, ctrl.toolError)
	mcpops.Mount(srv, reviews.DiffReviewClear, ctrl.ClearDiffReviews, ctrl.toolError)
}

type diffReviewsResult struct {
	Reviews []reviews.DiffReviewOut `json:"reviews"`
}

type clearDiffReviewsResult struct {
	Removed int `json:"removed" jsonschema:"How many reviews were removed; 0 when none matched."`
}

func (ctrl *DriveController) PutDiffReview(ctx context.Context, in reviews.DiffReviewPutIn) (reviews.DiffReviewOut, error) {
	r, err := ctrl.core.DiffReviews.Put(ctx, in)
	if err != nil {
		return reviews.DiffReviewOut{}, err
	}
	return reviews.OutOf(r), nil
}

func (ctrl *DriveController) GetDiffReviews(ctx context.Context, in reviews.DiffReviewGetIn) (diffReviewsResult, error) {
	list, err := ctrl.core.DiffReviews.Get(ctx, in.Caller, in.SessionID)
	if err != nil {
		return diffReviewsResult{}, err
	}
	out := diffReviewsResult{Reviews: make([]reviews.DiffReviewOut, 0, len(list))}
	for _, r := range list {
		out.Reviews = append(out.Reviews, reviews.OutOf(r))
	}
	return out, nil
}

func (ctrl *DriveController) ClearDiffReviews(ctx context.Context, in reviews.DiffReviewClearIn) (clearDiffReviewsResult, error) {
	n, err := ctrl.core.DiffReviews.Clear(ctx, in.Caller, in.SessionID, in.Author)
	if err != nil {
		return clearDiffReviewsResult{}, err
	}
	return clearDiffReviewsResult{Removed: n}, nil
}
