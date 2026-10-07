package usageanalytics

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	store "github.com/colonyops/hive/internal/store/usageanalytics"
	"github.com/colonyops/hive/pkg/logutils"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/mod/semver"
)

type localSink interface {
	WriteBatch(context.Context, []domain.Captured) error
	Prune(context.Context, time.Time) error
	Close() error
}

type Options struct {
	Enabled        bool
	DataDir        string
	Surface        string
	AppVersion     string
	ReleaseChannel string
}

// Service owns one bounded local queue. Delivery never determines product success.
type Service struct {
	mu        sync.RWMutex
	cutoff    atomic.Int64
	queue     chan domain.Captured
	pressure  chan struct{}
	done      chan struct{}
	cancel    context.CancelFunc
	closed    bool
	closeOnce sync.Once
	sink      localSink
	options   Options
	runID     string
	log       zerolog.Logger
}

// New returns a resource-free recorder when disabled or when the local store fails.
func New(ctx context.Context, logger zerolog.Logger, opts Options) *Service {
	log := logutils.Component(logger, "usageanalytics")
	if opts.ReleaseChannel == "" {
		opts.ReleaseChannel = releaseChannel(opts.AppVersion)
	}
	if !opts.Enabled {
		return &Service{log: log}
	}
	openCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	sink, err := store.Open(openCtx, opts.DataDir)
	if err != nil {
		log.Warn().Err(err).Msg("usage analytics unavailable")
		return &Service{log: log}
	}
	return newService(ctx, log, opts, sink)
}

func releaseChannel(version string) string {
	version = "v" + strings.TrimPrefix(version, "v")
	if !semver.IsValid(version) {
		return "development"
	}
	pre := semver.Prerelease(version)
	if pre == "" {
		return "stable"
	}
	if strings.HasPrefix(pre, "-beta.") {
		return "beta"
	}
	return "development"
}

func newService(ctx context.Context, logger zerolog.Logger, opts Options, sink localSink) *Service {
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &Service{queue: make(chan domain.Captured, 128), pressure: make(chan struct{}, 1), done: make(chan struct{}), cancel: cancel, sink: sink, options: opts, runID: uuid.NewString(), log: logger}
	go s.run(runCtx)
	return s
}

func (s *Service) DiscardBefore(cutoff time.Time) { s.cutoff.Store(cutoff.UnixNano()) }

func (s *Service) Active() bool {
	if s == nil || s.queue == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.closed
}

func (s *Service) Record(ctx context.Context, event domain.Event) {
	if !s.Active() || ctx.Err() != nil {
		return
	}
	if err := event.Validate(); err != nil {
		droppedEvents.Add(ctx, 1, localSinkAttribute)
		s.log.Warn().Msg("dropped invalid usage event")
		return
	}
	id, err := uuid.NewV7()
	if err != nil {
		droppedEvents.Add(ctx, 1, localSinkAttribute)
		s.log.Warn().Err(err).Msg("usage event ID failed")
		return
	}
	captured := domain.Captured{Event: event, ID: id.String(), RunID: s.runID, Surface: s.options.Surface, AppVersion: s.options.AppVersion, ReleaseChannel: s.options.ReleaseChannel}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.queue <- captured:
		acceptedEvents.Add(ctx, 1, localSinkAttribute)
		return
	default:
	}
	select {
	case s.pressure <- struct{}{}:
	default:
	}
	timer := time.NewTimer(25 * time.Millisecond)
	defer timer.Stop()
	select {
	case s.queue <- captured:
		acceptedEvents.Add(ctx, 1, localSinkAttribute)
	case <-ctx.Done():
		droppedEvents.Add(ctx, 1, localSinkAttribute)
	case <-timer.C:
		droppedEvents.Add(ctx, 1, localSinkAttribute)
		s.log.Warn().Msg("dropped usage event: queue full")
	}
}

// Close rejects new events, cancels in-flight I/O, then gives pending events a fresh two-second flush deadline.
func (s *Service) Close() {
	if s == nil || s.queue == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		s.mu.Unlock()
		<-s.done
	})
}

func (s *Service) run(ctx context.Context) {
	defer close(s.done)
	defer func() {
		if err := s.sink.Close(); err != nil {
			s.log.Warn().Err(err).Msg("close usage store failed")
		}
	}()
	flushTicker := time.NewTicker(10 * time.Second)
	dailyTicker := time.NewTicker(24 * time.Hour)
	defer flushTicker.Stop()
	defer dailyTicker.Stop()
	s.prune(ctx)
	batch := make([]domain.Captured, 0, 32)
	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if !s.write(flushCtx, batch) {
			if ctx.Err() != nil && flushCtx == ctx {
				return
			}
			droppedEvents.Add(flushCtx, int64(len(batch)), localSinkAttribute)
			s.log.Warn().Int("events", len(batch)).Msg("dropped usage batch")
		}
		batch = batch[:0]
	}
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case e := <-s.queue:
			batch = append(batch, e)
			if len(batch) == 32 {
				flush(ctx)
			}
		case <-flushTicker.C:
			flush(ctx)
		case <-s.pressure:
			flush(ctx)
		case <-dailyTicker.C:
			s.prune(ctx)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if len(batch) == 32 {
		flush(shutdownCtx)
	}
	for {
		select {
		case e := <-s.queue:
			batch = append(batch, e)
			if len(batch) == 32 {
				flush(shutdownCtx)
			}
		default:
			flush(shutdownCtx)
			return
		}
	}
}

func (s *Service) write(ctx context.Context, batch []domain.Captured) bool {
	retained := make([]domain.Captured, 0, len(batch))
	for _, event := range batch {
		if event.OccurredAt().UnixNano() > s.cutoff.Load() {
			retained = append(retained, event)
		}
	}
	if len(retained) == 0 {
		return true
	}
	batchSize.Record(ctx, int64(len(retained)), localSinkAttribute)
	for attempt := range 3 {
		started := time.Now()
		err := s.sink.WriteBatch(ctx, retained)
		writeDuration.Record(ctx, time.Since(started).Seconds(), localSinkAttribute)
		if err == nil {
			return true
		} else {
			failedBatches.Add(ctx, 1, localSinkAttribute)
			s.log.Warn().Err(err).Int("attempt", attempt+1).Msg("usage batch failed")
		}
		if attempt == 2 || ctx.Err() != nil {
			return false
		}
		timer := time.NewTimer(time.Duration(1<<attempt) * 50 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return false
		}
	}
	return false
}

func (s *Service) prune(ctx context.Context) {
	maintenanceCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.sink.Prune(maintenanceCtx, time.Now().UTC()); err != nil && ctx.Err() == nil {
		s.log.Warn().Err(err).Msg("usage retention failed")
	}
}
