package pipelinedb

import (
	"context"
	"fmt"
)

// NodeRunRecord is the JSON/Wails-friendly shape of a persisted node_run
// row, read back for the flows canvas's live status and RECENT list. It
// carries the same fields as NodeRunView (commit.go's write-side shape)
// plus EndedAt, which only exists once a run has actually been persisted —
// CommitBatch stamps it server-side, so the write-side NodeRunView has no
// use for it.
type NodeRunRecord struct {
	FlowID    string `json:"flowId"`
	NodeID    string `json:"nodeId"`
	OK        bool   `json:"ok"`
	InCount   int    `json:"inCount"`
	OutCount  int    `json:"outCount"`
	DropCount int    `json:"dropCount"`
	Err       string `json:"err"`
	DurMs     int64  `json:"durMs"`
	EndedAt   int64  `json:"endedAt"`
}

// NodeRuns returns up to limit of a flow's most recent node_run rows,
// newest first. The frontend canvas derives each node's latest status and a
// RECENT activity list from this single page rather than querying per-node.
func (db *DB) NodeRuns(ctx context.Context, flowID string, limit int) ([]NodeRunRecord, error) {
	rows, err := db.queries.ListNodeRunsByFlow(ctx, ListNodeRunsByFlowParams{
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
