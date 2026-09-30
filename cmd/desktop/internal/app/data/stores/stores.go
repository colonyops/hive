package stores

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
)

// Stores provides shared store instances and the transaction boundary for
// service writes that span aggregates.
type Stores struct {
	q *queries.DB

	ActivityEvents  *ActivityEventStore
	AgentSessions   *AgentSessionStore
	EventLog        *EventLogStore
	FeedClaims      *FeedClaimStore
	InboxItems      *InboxItemStore
	ItemSessions    *ItemSessionStore
	Jobs            *JobStore
	NodeKV          *NodeKVStore
	NodeRuns        *NodeRunStore
	OutputCommands  *OutputCommandStore
	Schedules       *ScheduleStore
	SourceHeads     *SourceHeadStore
	WebhookCaptures *WebhookCaptureStore
}

type Options struct {
	// Now supplies write timestamps; defaults to time.Now.
	Now func() time.Time
	// Logger is where a store reports a recoverable anomaly it skips rather
	// than fails on.
	Logger zerolog.Logger
}

func (o Options) withDefaults() Options {
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}

func New(q *queries.DB, opts Options) *Stores {
	opts = opts.withDefaults()

	heads := NewSourceHeadStore(q, opts)
	claims := NewFeedClaimStore(q, opts)
	kv := NewNodeKVStore(q, opts)
	runs := NewNodeRunStore(q, opts)
	commands := NewOutputCommandStore(q, opts)
	sessions := NewItemSessionStore(q, opts)

	// Ingest appends through EventLogStore, while commit and replay resolve
	// items through InboxItemStore, so the stores form a cycle.
	items := NewInboxItemStore(q, opts, heads, sessions)
	log := NewEventLogStore(q, opts, items, claims, kv, runs, commands)
	items.log = log

	return &Stores{
		q: q,

		ActivityEvents:  NewActivityEventStore(q, opts),
		AgentSessions:   NewAgentSessionStore(q, opts),
		EventLog:        log,
		FeedClaims:      claims,
		InboxItems:      items,
		ItemSessions:    sessions,
		Jobs:            NewJobStore(q, opts),
		NodeKV:          kv,
		NodeRuns:        runs,
		OutputCommands:  commands,
		Schedules:       NewScheduleStore(q, opts),
		SourceHeads:     heads,
		WebhookCaptures: NewWebhookCaptureStore(q, opts),
	}
}

// WithinTx joins an ambient transaction; only the outermost call commits or
// rolls back.
func (s *Stores) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return s.q.WithinTx(ctx, func(ctx context.Context, _ *queries.DB) error { return fn(ctx) })
}
