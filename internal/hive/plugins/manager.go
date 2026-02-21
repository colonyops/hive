package plugins

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/rs/zerolog/log"
)

const (
	// Default buffer sizes for channels
	defaultJobBufferSize    = 100
	defaultResultBufferSize = 100

	// Default number of background workers
	defaultWorkerCount = 3
)

// Manager manages plugin registration, lifecycle, and status fetching.
type Manager struct {
	plugins map[string]Plugin
	pool    *WorkerPool
	mu      sync.RWMutex

	// Background worker state
	collector     *StatusCollector
	jobs          chan Job
	results       chan Result
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	workerCount   int
	workerStarted bool
	workerStartMu sync.Mutex

	// Session state for background refresh
	sessionsMu     sync.RWMutex
	sessions       []*session.Session
	refreshTrigger chan struct{}
}

// NewManager creates a new plugin manager with a shared worker pool.
func NewManager(cfg config.PluginsConfig) *Manager {
	return &Manager{
		plugins:        make(map[string]Plugin),
		pool:           NewWorkerPool(cfg.ShellWorkers),
		collector:      NewStatusCollector(),
		jobs:           make(chan Job, defaultJobBufferSize),
		results:        make(chan Result, defaultResultBufferSize),
		refreshTrigger: make(chan struct{}, 1),
		workerCount:    defaultWorkerCount,
	}
}

// Register adds a plugin if it is available.
// Plugins that are not available (missing dependencies) are silently skipped.
func (m *Manager) Register(p Plugin) {
	if !p.Available() {
		log.Debug().Str("plugin", p.Name()).Msg("plugin not available, skipping")
		return
	}

	m.mu.Lock()
	m.plugins[p.Name()] = p
	m.mu.Unlock()

	log.Debug().Str("plugin", p.Name()).Msg("plugin registered")
}

// InitAll initializes all registered plugins.
// Errors are logged but do not stop initialization of other plugins.
func (m *Manager) InitAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, p := range m.plugins {
		if err := p.Init(ctx); err != nil {
			log.Warn().Err(err).Str("plugin", name).Msg("plugin initialization failed")
		}
	}
	return nil
}

// CloseAll closes all registered plugins and stops background workers.
func (m *Manager) CloseAll() {
	// Stop background workers first
	m.Stop()

	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, p := range m.plugins {
		if err := p.Close(); err != nil {
			log.Warn().Err(err).Str("plugin", name).Msg("plugin close failed")
		}
	}
}

// EnabledPlugins returns all registered (available) plugins.
func (m *Manager) EnabledPlugins() []Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// MergedCommands merges system, plugin, and user commands.
// Priority order: system (lowest) → plugins → user (highest).
func (m *Manager) MergedCommands(systemCmds, userCmds map[string]config.UserCommand) map[string]config.UserCommand {
	result := make(map[string]config.UserCommand)

	// 1. System defaults (lowest priority)
	maps.Copy(result, systemCmds)

	// 2. Plugin commands (middle priority)
	m.mu.RLock()
	for _, p := range m.plugins {
		maps.Copy(result, p.Commands())
	}
	m.mu.RUnlock()

	// 3. User commands (highest priority - always wins)
	maps.Copy(result, userCmds)

	return result
}

// Collector returns the status collector for reading cached statuses.
func (m *Manager) Collector() *StatusCollector {
	return m.collector
}

// StartBackgroundWorker starts background workers that fetch plugin statuses.
// Returns a channel that receives results as they complete.
// Call Stop() to shut down the workers.
func (m *Manager) StartBackgroundWorker(ctx context.Context, pollInterval time.Duration) <-chan Result {
	m.workerStartMu.Lock()
	defer m.workerStartMu.Unlock()

	if m.workerStarted {
		log.Warn().Msg("background workers already started")
		output := make(chan Result, defaultResultBufferSize)
		go m.resultForwarder(ctx, output)
		return output
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.workerStarted = true

	// Start workers
	for i := 0; i < m.workerCount; i++ {
		m.wg.Add(1)
		go m.worker(ctx, i)
	}

	// Start scheduler
	m.wg.Add(1)
	go m.scheduler(ctx, pollInterval)

	// Create output channel and start forwarder
	output := make(chan Result, defaultResultBufferSize)
	go m.resultForwarder(ctx, output)

	log.Debug().
		Int("workers", m.workerCount).
		Dur("pollInterval", pollInterval).
		Msg("background workers started")

	return output
}

// UpdateSessions updates the session list. A plugin refresh is only triggered
// when sessions are added or removed to avoid unnecessary API calls (e.g. GitHub).
func (m *Manager) UpdateSessions(sessions []*session.Session) {
	m.sessionsMu.Lock()
	changed := sessionsChanged(m.sessions, sessions)
	m.sessions = sessions
	m.sessionsMu.Unlock()

	if !changed {
		return
	}

	// Trigger immediate refresh (non-blocking)
	select {
	case m.refreshTrigger <- struct{}{}:
		log.Debug().Int("sessions", len(sessions)).Msg("refresh triggered (sessions changed)")
	default:
		// Refresh already pending
	}
}

// sessionsChanged reports whether the set of session IDs differs between old and new.
func sessionsChanged(old, new []*session.Session) bool {
	if len(old) != len(new) {
		return true
	}
	ids := make(map[string]struct{}, len(old))
	for _, s := range old {
		ids[s.ID] = struct{}{}
	}
	for _, s := range new {
		if _, ok := ids[s.ID]; !ok {
			return true
		}
	}
	return false
}

// Stop shuts down background workers and waits for them to finish.
func (m *Manager) Stop() {
	m.workerStartMu.Lock()
	defer m.workerStartMu.Unlock()

	if !m.workerStarted {
		return
	}

	if m.cancel != nil {
		m.cancel()
	}

	// Close jobs channel to signal workers to exit
	close(m.jobs)

	// Wait for workers to finish
	m.wg.Wait()

	// Close results channel
	close(m.results)

	m.workerStarted = false
	log.Debug().Msg("background workers stopped")
}

// scheduler runs the polling loop that enqueues jobs.
func (m *Manager) scheduler(ctx context.Context, pollInterval time.Duration) {
	defer m.wg.Done()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Initial fetch
	m.enqueueAllJobs(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.refreshTrigger:
			m.enqueueAllJobs(ctx)
		case <-ticker.C:
			m.enqueueAllJobs(ctx)
		}
	}
}

// enqueueAllJobs creates jobs for all plugin/session combinations.
func (m *Manager) enqueueAllJobs(ctx context.Context) {
	// Get current sessions
	m.sessionsMu.RLock()
	sessions := m.sessions
	m.sessionsMu.RUnlock()

	if len(sessions) == 0 {
		return
	}

	// Get plugins with status providers
	m.mu.RLock()
	var statusPlugins []Plugin
	for _, p := range m.plugins {
		if p.StatusProvider() != nil {
			statusPlugins = append(statusPlugins, p)
		}
	}
	m.mu.RUnlock()

	if len(statusPlugins) == 0 {
		return
	}

	log.Debug().
		Int("plugins", len(statusPlugins)).
		Int("sessions", len(sessions)).
		Msg("enqueueing plugin fetch jobs")

	// Enqueue jobs for each plugin/session combination
	for _, p := range statusPlugins {
		for _, sess := range sessions {
			select {
			case <-ctx.Done():
				return
			case m.jobs <- Job{
				PluginName: p.Name(),
				SessionID:  sess.ID,
				Session:    sess,
			}:
			}
		}
	}
}

// worker processes jobs from the jobs channel.
func (m *Manager) worker(ctx context.Context, id int) {
	defer m.wg.Done()

	log.Debug().Int("workerID", id).Msg("worker started")

	for {
		select {
		case <-ctx.Done():
			log.Debug().Int("workerID", id).Msg("worker stopping (context cancelled)")
			return
		case job, ok := <-m.jobs:
			if !ok {
				log.Debug().Int("workerID", id).Msg("worker stopping (channel closed)")
				return
			}
			m.processJob(ctx, job)
		}
	}
}

// processJob executes a single status fetch job.
func (m *Manager) processJob(ctx context.Context, job Job) {
	// Get the plugin
	m.mu.RLock()
	plugin, ok := m.plugins[job.PluginName]
	m.mu.RUnlock()

	if !ok {
		return
	}

	provider := plugin.StatusProvider()
	if provider == nil {
		return
	}

	// Use the worker pool for rate limiting
	err := m.pool.RunContext(ctx, func() {
		// Call RefreshStatus with a single session
		sessions := []*session.Session{job.Session}
		statuses, err := provider.RefreshStatus(ctx, sessions, m.pool)

		result := Result{
			PluginName: job.PluginName,
			SessionID:  job.SessionID,
			Err:        err,
		}

		if err == nil {
			status, ok := statuses[job.SessionID]
			if !ok {
				// Plugin returned no status for this session.
				// Don't send a result — preserve any existing cached status.
				return
			}
			result.Status = status
		}

		// Send result (non-blocking with select)
		select {
		case <-ctx.Done():
			return
		case m.results <- result:
		}
	})
	if err != nil {
		// Context cancelled while waiting for pool
		log.Debug().
			Str("plugin", job.PluginName).
			Str("session", job.SessionID).
			Msg("job cancelled while waiting for pool")
	}
}

// resultForwarder reads from internal results channel, updates collector, and forwards to output.
func (m *Manager) resultForwarder(ctx context.Context, output chan<- Result) {
	defer close(output)

	for {
		select {
		case <-ctx.Done():
			return
		case result, ok := <-m.results:
			if !ok {
				return
			}

			// Store in collector
			if result.Err == nil {
				m.collector.Set(result.PluginName, result.SessionID, result.Status)
			}

			// Forward to output
			select {
			case <-ctx.Done():
				return
			case output <- result:
			}
		}
	}
}

// RefreshAllStatus fetches status from all plugins for the given sessions.
// Returns a map of plugin name -> session ID -> status.
//
// Deprecated: Use StartBackgroundWorker for non-blocking status fetching.
func (m *Manager) RefreshAllStatus(ctx context.Context, sessions []*session.Session) map[string]map[string]Status {
	results := make(map[string]map[string]Status)

	m.mu.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	m.mu.RUnlock()

	log.Debug().Int("pluginCount", len(plugins)).Msg("RefreshAllStatus starting")

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, p := range plugins {
		sp := p.StatusProvider()
		if sp == nil {
			continue
		}

		wg.Add(1)
		go func(plugin Plugin, provider StatusProvider) {
			defer wg.Done()

			log.Debug().Str("plugin", plugin.Name()).Msg("plugin RefreshStatus starting")
			statuses, err := provider.RefreshStatus(ctx, sessions, m.pool)
			log.Debug().Str("plugin", plugin.Name()).Int("count", len(statuses)).Msg("plugin RefreshStatus complete")
			if err != nil {
				log.Debug().Err(err).Str("plugin", plugin.Name()).Msg("status refresh failed")
				return
			}

			mu.Lock()
			results[plugin.Name()] = statuses
			mu.Unlock()
		}(p, sp)
	}

	wg.Wait()
	log.Debug().Int("resultCount", len(results)).Msg("RefreshAllStatus complete")
	return results
}

// Pool returns the shared worker pool for plugins to use.
func (m *Manager) Pool() *WorkerPool {
	return m.pool
}

// Get returns a plugin by name, or nil if not found.
func (m *Manager) Get(name string) Plugin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plugins[name]
}
