package drive

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/observe"
)

// No instrument here is labelled by space or path: both are per user and
// unbounded, so they ride on the caller's span instead
// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
var (
	meter  = observe.Meter("/internal/hive/drive")
	tracer = observe.Tracer("/internal/hive/drive")

	// A drive operation is a few file system calls, so it takes tens of
	// microseconds to a few milliseconds; the SDK default buckets start at 5s.
	operationDuration = observe.Must(meter.Float64Histogram(
		"drive.operation.duration",
		metric.WithDescription("Duration of one drive read or write, by operation, actor, and result."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5,
		),
	))

	// A scan head-reads every document in a space, so it is what a large
	// space costs the first time anyone opens it.
	scanDuration = observe.Must(meter.Float64Histogram(
		"drive.scan.duration",
		metric.WithDescription("Duration of a whole-space scan from disk."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
		),
	))

	scanEntries = observe.Must(meter.Int64Histogram(
		"drive.scan.entries",
		metric.WithDescription("Documents and folders a whole-space scan indexed."),
		metric.WithUnit("{entry}"),
		metric.WithExplicitBucketBoundaries(10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 50000),
	))

	// What answers whether the watcher earns its keep: unchanged is the echo
	// of the drive's own writes, published is a change from outside the app.
	watchChanges = observe.Must(meter.Int64Counter(
		"drive.watch.changes",
		metric.WithDescription("Debounced file system changes the drive watcher applied, by outcome."),
	))
)

const (
	opRead           = "read"
	opWrite          = "write"
	opCreateFolder   = "create_folder"
	opMove           = "move"
	opDelete         = "delete"
	opSearch         = "search"
	opLoadCanvas     = "load_canvas"
	opUpsertBlocks   = "upsert_blocks"
	opRemoveBlock    = "remove_block"
	opSetFrontmatter = "set_frontmatter"
	opClearCanvas    = "clear_canvas"
	opListCanvases   = "list_canvases"

	actorAgent = "agent"
	actorUser  = "user"
	// A read carries no author, so it has no actor.
	actorNone = ""

	resultOK = "ok"
	// refused is a call the drive turned down by its rules: a missing
	// document, a bad path, a target that exists. error is everything else,
	// a file system failure the caller could not have avoided.
	resultRefused = "refused"
	resultError   = "error"

	watchPublished = "published"
	watchUnchanged = "unchanged"
	watchResync    = "resync"
)

// Span attributes, not metric labels, which is why a space key and a path
// are safe here.
const (
	attrSpace       = "drive.space"
	attrPath        = "drive.path"
	attrTarget      = "drive.target"
	attrBytes       = "drive.bytes"
	attrBlocks      = "drive.blocks"
	attrHits        = "drive.hits"
	attrWatchDirs   = "drive.watch.dirs"
	attrWatchSpaces = "drive.watch.spaces"
	attrWatchResync = "drive.watch.resync"
)

// Built once: metric.WithAttributes allocates, and this is every call's path.
var (
	operationAttrs = func() map[[3]string]metric.MeasurementOption {
		ops := []string{
			opRead, opWrite, opCreateFolder, opMove, opDelete, opSearch, opLoadCanvas,
			opUpsertBlocks, opRemoveBlock, opSetFrontmatter, opClearCanvas, opListCanvases,
		}
		out := map[[3]string]metric.MeasurementOption{}
		for _, op := range ops {
			for _, actor := range []string{actorAgent, actorUser, actorNone} {
				for _, result := range []string{resultOK, resultRefused, resultError} {
					attrs := []attribute.KeyValue{
						attribute.String("drive.operation", op),
						attribute.String("result", result),
					}
					if actor != actorNone {
						attrs = append(attrs, attribute.String("drive.actor", actor))
					}
					out[[3]string{op, actor, result}] = metric.WithAttributes(attrs...)
				}
			}
		}
		return out
	}()

	watchAttrs = map[string]metric.AddOption{
		watchPublished: metric.WithAttributes(attribute.String("outcome", watchPublished)),
		watchUnchanged: metric.WithAttributes(attribute.String("outcome", watchUnchanged)),
		watchResync:    metric.WithAttributes(attribute.String("outcome", watchResync)),
	}
)

func actorOf(author drive.Author) string {
	if author.Session != "" || author.HiveSession != "" {
		return actorAgent
	}
	return actorUser
}

func resultOf(err error) string {
	switch {
	case err == nil:
		return resultOK
	case errors.Is(err, drive.ErrNotFound), errors.Is(err, drive.ErrInvalidPath),
		errors.Is(err, drive.ErrInvalidSpace), errors.Is(err, drive.ErrExists),
		errors.Is(err, drive.ErrNotEmpty), errors.Is(err, drive.ErrNotWritable),
		errors.Is(err, drive.ErrAnchorNotFound), errors.Is(err, drive.ErrInvalidFrontmatter):
		return resultRefused
	default:
		return resultError
	}
}

// recordOp pairs with a named error return: `defer recordOp(ctx, op, actor,
// time.Now(), &err)`.
func recordOp(ctx context.Context, op, actor string, start time.Time, err *error) {
	operationDuration.Record(ctx, time.Since(start).Seconds(), operationAttrs[[3]string{op, actor, resultOf(*err)}])
}

// annotate files what an operation touched on the span the caller carries,
// an MCP tool call or a UI command. The drive opens no span of its own for a
// local file operation; it is neither a trigger nor a wait.
func annotate(ctx context.Context, space drive.Space, path string, extra ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(attribute.String(attrSpace, space.Key), attribute.String(attrPath, path))
	span.SetAttributes(extra...)
}

func recordScan(start time.Time, root drive.Entry) int {
	n := countEntries(root)
	ctx := context.Background()
	scanDuration.Record(ctx, time.Since(start).Seconds())
	scanEntries.Record(ctx, int64(n))
	return n
}

func countEntries(entry drive.Entry) int {
	n := len(entry.Children)
	for _, child := range entry.Children {
		n += countEntries(child)
	}
	return n
}
