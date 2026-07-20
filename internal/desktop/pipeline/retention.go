package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/rs/zerolog"
)

// DefaultRetentionInterval is deliberately infrequent: retention only touches
// local SQLite history and is safe to defer, while producer/worker work stays
// responsive under normal desktop use.
const DefaultRetentionInterval = 5 * time.Minute

// RetentionStore is the subset of the pipeline database required by
// Maintenance. *pipelinedb.DB satisfies it.
type RetentionStore interface {
	Prune(ctx context.Context, enabledConsumers []string, policy pipelinedb.RetentionPolicy) (pipelinedb.RetentionResult, error)
}

// Maintenance periodically bounds pipeline history. It resolves enabled flow
// IDs at each tick rather than retaining an initial list, so deploys and
// external flow-file edits immediately affect the event-log safety boundary.
type Maintenance struct {
	db       RetentionStore
	flows    FlowLister
	policy   pipelinedb.RetentionPolicy
	interval time.Duration
	logger   zerolog.Logger

	mu       sync.Mutex
	started  bool
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

// NewMaintenance constructs the background retention loop. Callers must pass
// a positive interval because time.NewTicker rejects zero and negative durations.
func NewMaintenance(db RetentionStore, flows FlowLister, policy pipelinedb.RetentionPolicy, interval time.Duration, logger zerolog.Logger) *Maintenance {
	return &Maintenance{
		db:       db,
		flows:    flows,
		policy:   policy,
		interval: interval,
		logger:   logger,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins the periodic maintenance loop. The first run happens on the
// first interval rather than app startup, avoiding needless write contention
// while the frontend restores its durable flow consumers.
func (m *Maintenance) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	go func() {
		defer close(m.done)
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-ticker.C:
				m.Tick(context.Background())
			}
		}
	}()
}

// Stop halts maintenance and waits for its goroutine to exit, so callers can
// close the SQLite database without a concurrent retention query.
func (m *Maintenance) Stop() {
	m.mu.Lock()
	started := m.started
	m.mu.Unlock()
	if !started {
		return
	}
	m.stopOnce.Do(func() { close(m.stop) })
	<-m.done
}

// Tick applies one retention pass. Failures are logged and retried on the
// next interval; maintenance must never terminate the desktop backend.
func (m *Maintenance) Tick(ctx context.Context) {
	enabled := make([]string, 0)
	for _, f := range m.flows.List() {
		if f.Enabled {
			enabled = append(enabled, f.ID)
		}
	}
	if _, err := m.db.Prune(ctx, enabled, m.policy); err != nil {
		m.logger.Warn().Err(err).Msg("pipeline retention: prune failed")
	}
}
