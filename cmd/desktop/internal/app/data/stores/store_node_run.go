package stores

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

type NodeRunStore struct {
	q *queries.DB
}

func NewNodeRunStore(q *queries.DB, _ Options) *NodeRunStore {
	return &NodeRunStore{q: q}
}

func (s *NodeRunStore) List(ctx context.Context, flowID string, limit int) ([]NodeRunRecord, error) {
	rows, err := s.q.Ctx(ctx).ListNodeRunsByFlow(ctx, queries.ListNodeRunsByFlowParams{
		FlowID: flowID,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("listing node runs for flow %q: %w", flowID, err)
	}

	runs := make([]NodeRunRecord, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, NodeRunRecord{
			FlowID:    row.FlowID,
			NodeID:    row.NodeID,
			OK:        row.Ok != 0,
			InCount:   int(row.InCount),
			OutCount:  int(row.OutCount),
			DropCount: int(row.DropCount),
			Err:       row.Err.String,
			DurMs:     row.DurMs,
			EndedAt:   row.EndedAt,
		})
	}
	return runs, nil
}

// endedAt comes from the caller so all node runs in one commit share a
// timestamp.
func (s *NodeRunStore) Insert(ctx context.Context, run models.NodeRun, endedAt int64) error {
	var errCol sql.NullString
	if run.Err != "" {
		errCol = sql.NullString{String: run.Err, Valid: true}
	}
	return wrap("inserting node run", s.q.Ctx(ctx).InsertNodeRun(ctx, queries.InsertNodeRunParams{
		FlowID:    run.FlowID,
		NodeID:    run.NodeID,
		Ok:        boolToInt64(run.OK),
		InCount:   int64(run.InCount),
		OutCount:  int64(run.OutCount),
		DropCount: int64(run.DropCount),
		Err:       errCol,
		EndedAt:   endedAt,
		DurMs:     run.DurMs,
	}))
}
