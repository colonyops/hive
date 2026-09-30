// Package ingest is the pipeline's producer side: the poll loop that turns
// configured source connectors into event_log rows, and the resolver that
// turns the current flow set into the connector instances it drives.
//
// Delivery — reading the log, routing it, committing a consumer's offset —
// belongs to internal/app/runtime; this package only appends.
package ingest

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/models"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/flow"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

type Msg = models.Msg

// Sources is what a producer tick needs from the connector registry. Declared
// here rather than exported from the registry because a package's dependency
// is exactly the methods it calls.
type Sources interface {
	// PullInstances is every enabled pull-mode connector instance across the
	// current flow set. It is called once per tick — rather than fixed at
	// construction — so a source node added to or removed from a flow takes
	// effect without a restart.
	PullInstances(ctx context.Context) []connector.Instance
	// Prefetch offers the tick's instances to any connector that declared
	// CapBatchPrefetch, before any of them is drained.
	Prefetch(ctx context.Context, instances []connector.Instance) error
}

// FlowLister is the subset of *flow.FlowStore this package needs: the current
// set of loaded flows.
type FlowLister interface {
	List() []flow.Flow
}

// Ingester atomically updates inbox state, source head, and event log for one
// observation.
type Ingester interface {
	IngestObservation(ctx context.Context, classifier models.Classifier, p stores.IngestObservationParams) (stores.IngestResult, error)
}

// SnapshotAppender persists authoritative source state for feed replay.
type SnapshotAppender interface {
	AppendSnapshot(ctx context.Context, topic, sourceKind, sourceScope string, items []models.SnapshotItem) (offset int64, err error)
}

type SourceHeads interface {
	ListActiveKeys(ctx context.Context, id stores.SourceIdentity) ([]string, error)
	Payload(ctx context.Context, topic, key string) ([]byte, error)
	Delete(ctx context.Context, topic, key string) error
}
