package mcpsrv

import (
	"context"

	"github.com/colonyops/hive/internal/ops"
)

type refreshResult struct {
	Sources  int `json:"sources"  jsonschema:"Number of sources that ran this tick."`
	Appended int `json:"appended" jsonschema:"Number of inbox items appended across all sources."`
	Failed   int `json:"failed"   jsonschema:"Number of sources that failed this tick."`
}

func (ctrl *Controller) RefreshSources(ctx context.Context, _ ops.NoInput) (refreshResult, error) {
	sum, err := ctrl.core.RefreshSources(ctx)
	if err != nil {
		return refreshResult{}, err
	}
	return refreshResult{Sources: sum.Sources, Appended: sum.Appended, Failed: sum.Failed}, nil
}
