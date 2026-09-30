package app

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/tmuxcc"
)

type hiveSessionWindowSource struct {
	terminals *tmuxcc.Manager
}

func (s hiveSessionWindowSource) ListSessionWindows(ctx context.Context, slugs []string) (map[string][]dispatch.SessionWindowRef, error) {
	windows, err := s.terminals.ListIndexedWindows(ctx, slugs)
	if err != nil {
		return nil, err
	}
	refs := make(map[string][]dispatch.SessionWindowRef, len(windows))
	for slug, items := range windows {
		for _, item := range items {
			refs[slug] = append(refs[slug], dispatch.SessionWindowRef{
				ID:    item.ID,
				Index: item.Index,
				Name:  item.Name,
			})
		}
	}
	return refs, nil
}
