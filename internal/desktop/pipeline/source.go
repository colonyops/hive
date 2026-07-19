// Package pipeline is the desktop pipeline's producer side: the Source seam
// that turns a configured data source into event_log rows, and the Producer
// that drives it on a poll tick. Delivery (reading the log, committing a
// consumer's offset) is exposed to the frontend by desktop/pipelineservice.go;
// this package only appends.
package pipeline

import (
	"context"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
)

// Msg is the pipeline's generic log record. It is pipelinedb.Msg verbatim —
// Source implementations build one per item and Producer appends it as-is,
// so there is no separate wire type to keep in sync.
type Msg = pipelinedb.Msg

// Source produces the current state of one data source as a sequence of
// Msg, calling emit once per item. Produce is called synchronously from a
// Producer tick and returns once every current item has been emitted, or
// once fetching or an emit call fails. Implementations should set:
//   - Topic: "source:" + the source's stable ID, so consumers can filter by
//     source.
//   - Key: the item's stable identity, so pipelinedb.Compact's per-key
//     compaction collapses repeated appends of the same item to its latest
//     value.
//   - Payload: the item, JSON-encoded.
type Source interface {
	Produce(ctx context.Context, emit func(Msg) error) error
}

// SourceLister resolves the current set of sources to poll, keyed by a
// stable ID (used as the log topic and for logging). Producer calls it once
// per tick — rather than fixing the set at construction — so sources added
// to or removed from config take effect without a restart, mirroring how
// feed.Poller re-reads profiles/sources on every tick.
type SourceLister func(ctx context.Context) (map[string]Source, error)

// Appender is the subset of *pipelinedb.DB a Producer needs. Tests can
// substitute a fake to observe append calls directly; most tests still use
// a real pipelinedb.DB via t.TempDir(), which is cheap and exercises the
// real monotonic-offset contract.
type Appender interface {
	Append(ctx context.Context, topic, key string, payload []byte) (int64, error)
}
