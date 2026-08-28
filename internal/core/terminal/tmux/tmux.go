// Package tmux implements terminal integration for tmux.
package tmux

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/colonyops/hive/internal/core/terminal/content"
	"github.com/colonyops/hive/internal/core/terminal/process"
	"github.com/colonyops/hive/internal/core/terminal/status"
)

// contentCheckInterval is the minimum time between Tier 3 content-capture
// classification attempts for the same pane during RefreshCache.
const contentCheckInterval = 5 * time.Second

// defaultMissingTolerance is how many consecutive list-panes failures this
// integration tolerates (serving the stale cache) before publishing
// StatusMissing. Production wiring overrides it via WithMissingTolerance from
// terminal.status.confirm.missing.polls.
const defaultMissingTolerance = 2

// Integration implements terminal.Integration for tmux.
type Integration struct {
	mu                 sync.RWMutex
	refreshMu          sync.Mutex // prevents concurrent RefreshCache runs
	cache              map[string]*sessionCache
	cacheTime          time.Time
	tracker            *status.Tracker
	refreshGeneration  uint64
	refreshFailures    int
	missingTolerance   int
	limiters           map[string]*terminal.RateLimiter
	contentLimiters    map[string]*terminal.RateLimiter // per-pane Tier 3 rate limiter
	commander          Commander
	classifier         *classifier.Classifier
	classCache         *classifier.Cache
	processReader      process.ProcessReader
	lister             PaneLister
	capture            classifier.ContentCapture
	recorder           CaptureRecorder
	beforePollGateLock func() // deterministic seam for refresh/GetStatus overlap tests
}

// sessionCache holds all panes for a single tmux session.
type sessionCache struct {
	panes []cachedPane
}

// cachedPane combines pane identity, classification output, and polling state.
type cachedPane struct {
	input              classifier.PaneInput
	result             classifier.Result
	state              paneState
	processFingerprint int64
	pollMu             *sync.Mutex
}

// agentPanes returns panes classified as agents.
func (sc *sessionCache) agentPanes() []cachedPane {
	if sc == nil {
		return nil
	}
	agents := make([]cachedPane, 0, len(sc.panes))
	for _, pane := range sc.panes {
		if pane.result.IsAgent {
			agents = append(agents, pane)
		}
	}
	return agents
}

// findPane returns the pane matching paneID.
func (sc *sessionCache) findPane(paneID string) *cachedPane {
	if sc == nil || paneID == "" {
		return nil
	}
	for i := range sc.panes {
		if sc.panes[i].input.PaneID == paneID {
			return &sc.panes[i]
		}
	}
	return nil
}

func (sc *sessionCache) findAgentPane(paneID string) *cachedPane {
	pane := sc.findPane(paneID)
	if pane == nil || !pane.result.IsAgent {
		return nil
	}
	return pane
}

func (sc *sessionCache) findAgentPaneByWindow(windowIndex string) *cachedPane {
	if sc == nil || windowIndex == "" {
		return nil
	}
	for i := range sc.panes {
		pane := &sc.panes[i]
		if pane.result.IsAgent && pane.input.WindowIndex == windowIndex {
			return pane
		}
	}
	return nil
}

// bestAgentPane returns the highest-activity agent pane.
func (sc *sessionCache) bestAgentPane() *cachedPane {
	if sc == nil {
		return nil
	}
	var best *cachedPane
	for i := range sc.panes {
		pane := &sc.panes[i]
		if !pane.result.IsAgent {
			continue
		}
		if best == nil || pane.input.Activity > best.input.Activity {
			best = pane
		}
	}
	return best
}

// Option configures a tmux Integration.
type Option func(*Integration)

// WithCaptureRecorder records fresh agent-pane captures using recorder.
func WithCaptureRecorder(recorder CaptureRecorder) Option {
	return func(integration *Integration) {
		integration.recorder = recorder
	}
}

// WithCommander uses commander for tmux discovery, pane listing, and capture.
// Embedders can supply an absolute binary, environment, or socket selection
// without changing the process-wide environment.
func WithCommander(commander Commander) Option {
	return func(integration *Integration) {
		if commander != nil {
			integration.commander = commander
		}
	}
}

// WithStatusOptions configures the debounce policies for this integration's
// status tracker (production config path).
func WithStatusOptions(opts status.Options) Option {
	return func(integration *Integration) {
		integration.tracker = status.NewTracker(assess.NewEngine(), opts)
	}
}

// WithMissingTolerance sets how many consecutive list-panes failures
// RefreshCache tolerates (serving the stale cache) before publishing
// StatusMissing. n polls tolerates n-1 consecutive failures, matching
// terminal.status.confirm.missing.polls semantics.
func WithMissingTolerance(n int) Option {
	return func(integration *Integration) {
		if n > 0 {
			integration.missingTolerance = n
		}
	}
}

// NewFromPreviewMatchers creates the production tmux integration from config
// matchers. Tool names for process detection are derived automatically from
// the pattern strings (e.g. "^pi$" → "pi"), so callers only need to pass
// the single PreviewWindowMatcher slice from config.
func NewFromPreviewMatchers(previewMatchers []string, options ...Option) *Integration {
	reader := process.OSReader{}
	integration := newIntegration(reader)
	for _, option := range options {
		option(integration)
	}

	capture := TmuxCapture{commander: integration.commander}
	agentNames := classifier.ToolNamesFromPatterns(previewMatchers)
	integration.classifier = classifier.New(classifier.TitlePatternsFromConfig(previewMatchers, agentNames), reader, capture, content.NewScorer())
	integration.lister = TmuxPaneLister{commander: integration.commander}
	integration.capture = capture
	return integration
}

// New creates a new tmux integration.
func New(cls *classifier.Classifier, lister PaneLister) *Integration {
	return NewWithReader(cls, lister, process.OSReader{})
}

// NewWithReader creates a tmux integration with explicit dependencies for tests.
func NewWithReader(cls *classifier.Classifier, lister PaneLister, reader process.ProcessReader) *Integration {
	if reader == nil {
		reader = process.OSReader{}
	}
	integration := newIntegration(reader)
	capture := TmuxCapture{commander: integration.commander}
	if cls == nil {
		cls = classifier.New(nil, reader, capture, nil)
	}
	if lister == nil {
		lister = TmuxPaneLister{commander: integration.commander}
	}
	integration.classifier = cls
	integration.lister = lister
	integration.capture = capture
	return integration
}

func newIntegration(reader process.ProcessReader) *Integration {
	return &Integration{
		cache:            make(map[string]*sessionCache),
		tracker:          status.NewTracker(assess.NewEngine(), status.DefaultOptions()),
		limiters:         make(map[string]*terminal.RateLimiter),
		contentLimiters:  make(map[string]*terminal.RateLimiter),
		commander:        execCommander{},
		classCache:       classifier.NewCache(),
		processReader:    reader,
		missingTolerance: defaultMissingTolerance,
	}
}

// Classifier returns the pane classifier used by this integration.
func (t *Integration) Classifier() *classifier.Classifier { return t.classifier }

// Name returns "tmux".
func (t *Integration) Name() string { return "tmux" }

// Available returns true if tmux is installed and accessible.
func (t *Integration) Available() bool {
	return t.commander != nil && t.commander.Available()
}

// RefreshCache updates cached pane classifications. Call once per poll cycle.
// A TryLock guard ensures that if a previous refresh is still running (e.g.
// because Tier 3 capture-pane calls are slow), the new call returns immediately
// rather than stacking up concurrent tmux subprocess storms.
func (t *Integration) RefreshCache() {
	if !t.refreshMu.TryLock() {
		// A refresh is already in progress; skip this cycle.
		log.Debug().Msg("tmux RefreshCache skipped: previous refresh still running")
		return
	}
	defer t.refreshMu.Unlock()

	// Build a process-tree snapshot once for this refresh cycle so all pane
	// classifications share one OS call instead of one per pane.
	snapshotCls := t.classifier.WithReader(process.NewSnapshotReader(t.processReader))

	panes, err := t.lister.ListAllPanes()
	if err != nil {
		t.handleRefreshFailure(err)
		return
	}

	type paneSnapshot struct {
		sessionName        string
		paneID             string
		pid                int64
		processFingerprint int64
		state              paneState
		pollMu             *sync.Mutex
	}

	// Every cached pane owns a poll mutex. Initialize legacy/test-created nil
	// entries while holding the cache lock so concurrent GetStatus calls cannot
	// create a different mutex for the same pane.
	t.mu.Lock()
	oldStates := make(map[string]paneSnapshot)
	for sessionName, sc := range t.cache {
		for i := range sc.panes {
			pane := &sc.panes[i]
			if pane.pollMu == nil {
				pane.pollMu = &sync.Mutex{}
			}
			key := paneKey(sessionName, pane.input.PaneID)
			oldStates[key] = paneSnapshot{
				sessionName:        sessionName,
				paneID:             pane.input.PaneID,
				pid:                pane.input.PanePID,
				processFingerprint: pane.processFingerprint,
				state:              pane.state,
				pollMu:             pane.pollMu,
			}
		}
	}
	t.mu.Unlock()

	newCache := make(map[string]*sessionCache)
	activePaneIDs := make(map[string]bool, len(panes))
	activeKeys := make(map[string]bool, len(panes))
	reusedPanes := make(map[string]paneSnapshot)
	replacedKeys := make(map[string]bool)
	for _, input := range panes {
		if input.SessionName == "" || input.PaneID == "" {
			continue
		}
		activePaneIDs[input.PaneID] = true
		key := paneKey(input.SessionName, input.PaneID)
		activeKeys[key] = true

		fingerprint := t.processFingerprint(input.PanePID)
		previous, existed := oldStates[key]
		sameProcess := existed && previous.pid == input.PanePID &&
			(previous.processFingerprint == 0 || previous.processFingerprint == fingerprint)
		if sameProcess {
			reusedPanes[key] = previous
		} else if existed {
			replacedKeys[key] = true
			// A replacement must be eligible for Tier 3 classification now,
			// rather than inheriting the old process's classification cadence.
			t.mu.Lock()
			delete(t.contentLimiters, key)
			t.mu.Unlock()
		}

		result, ok := t.classCache.Get(input.PaneID, fingerprint)
		if !ok {
			// Gate Tier 3 (content capture) behind a per-pane rate limiter so
			// we never spawn more than one capture-pane per pane per interval.
			// On the first call Allow() returns true; subsequent calls within
			// contentCheckInterval use only Tiers 1 and 2.
			if t.contentLimiterAllow(key) {
				result = snapshotCls.Classify(context.Background(), input)
			} else {
				result = snapshotCls.ClassifyStable(input)
			}
			if result.StableForProcessCache() {
				t.classCache.Set(input.PaneID, fingerprint, result)
			}
		}

		pollMu := previous.pollMu
		if pollMu == nil {
			pollMu = &sync.Mutex{}
		}
		entry := cachedPane{
			input:              input,
			result:             result,
			processFingerprint: fingerprint,
			pollMu:             pollMu,
		}
		if sameProcess {
			entry.state = previous.state
		}
		sc := newCache[input.SessionName]
		if sc == nil {
			sc = &sessionCache{}
			newCache[input.SessionName] = sc
		}
		sc.panes = append(sc.panes, entry)
	}

	t.classCache.Prune(activePaneIDs)

	// GetStatus takes pollMu before re-entering t.mu. Acquire every old pane's
	// pollMu without holding t.mu, then publish and prune while those gates are
	// held. This lets in-flight observations finish first and prevents waiting
	// observations for removed/replaced panes from landing after pruning.
	oldGates := make(map[string]*sync.Mutex, len(oldStates))
	for key, snapshot := range oldStates {
		oldGates[key] = snapshot.pollMu
	}
	unlockPollGates := t.lockPollGates(oldGates)
	defer unlockPollGates()

	t.mu.Lock()
	// The early snapshot may predate an in-flight capture. Now that every old
	// pane is quiescent, copy the latest state for processes being reused.
	for _, previous := range reusedPanes {
		currentSession := t.cache[previous.sessionName]
		newSession := newCache[previous.sessionName]
		if currentSession == nil || newSession == nil {
			continue
		}
		currentPane := currentSession.findPane(previous.paneID)
		newPane := newSession.findPane(previous.paneID)
		if currentPane != nil && newPane != nil && currentPane.pollMu == previous.pollMu {
			newPane.state = currentPane.state
		}
	}

	t.cache = newCache
	t.cacheTime = time.Now()
	t.refreshFailures = 0
	t.refreshGeneration++
	for key := range replacedKeys {
		delete(t.limiters, key)
	}
	t.prunePaneKeysLocked(activeKeys)
	t.mu.Unlock()

	for key := range replacedKeys {
		t.tracker.Reset(key)
	}
	t.tracker.Prune(activeKeys)
}

// handleRefreshFailure applies the transport's missing-tolerance policy to a
// list-panes failure. Failures below missingTolerance serve the existing
// cache as-is (bumping cacheTime so the DiscoverSession/DiscoverAllPanes
// freshness gates tolerate exactly one served-stale window) rather than
// flashing every pane to missing on a single transient hiccup. Reaching
// missingTolerance clears the cache and prunes per-pane state.
func (t *Integration) handleRefreshFailure(err error) {
	t.mu.Lock()
	t.refreshFailures++
	failures := t.refreshFailures
	if failures < t.missingTolerance {
		t.cacheTime = time.Now()
		t.mu.Unlock()
		log.Debug().Err(err).Int("failures", failures).Msg("tmux list-panes failed, serving stale cache")
		return
	}

	oldGates := make(map[string]*sync.Mutex)
	for sessionName, sc := range t.cache {
		for i := range sc.panes {
			pane := &sc.panes[i]
			if pane.pollMu == nil {
				pane.pollMu = &sync.Mutex{}
			}
			oldGates[paneKey(sessionName, pane.input.PaneID)] = pane.pollMu
		}
	}
	t.mu.Unlock()

	unlockPollGates := t.lockPollGates(oldGates)
	defer unlockPollGates()

	t.mu.Lock()
	t.cache = make(map[string]*sessionCache)
	t.cacheTime = time.Time{}
	t.prunePaneKeysLocked(map[string]bool{})
	t.mu.Unlock()

	t.tracker.Prune(map[string]bool{})
	log.Debug().Err(err).Int("failures", failures).Msg("tmux list-panes failed, clearing cache")
}

// lockPollGates quiesces pane observations without holding the cache lock.
// Sorting makes multi-pane acquisition deterministic; pointer deduplication
// avoids deadlock if malformed cache data aliases one gate across pane keys.
func (t *Integration) lockPollGates(gates map[string]*sync.Mutex) func() {
	keys := make([]string, 0, len(gates))
	for key := range gates {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if t.beforePollGateLock != nil {
		t.beforePollGateLock()
	}

	locked := make([]*sync.Mutex, 0, len(keys))
	seen := make(map[*sync.Mutex]bool, len(keys))
	for _, key := range keys {
		gate := gates[key]
		if gate == nil || seen[gate] {
			continue
		}
		gate.Lock()
		seen[gate] = true
		locked = append(locked, gate)
	}

	return func() {
		for i := len(locked) - 1; i >= 0; i-- {
			locked[i].Unlock()
		}
	}
}

// contentLimiterAllow returns true if Tier 3 content capture is allowed for
// the given pane key at this moment, and records the attempt. The first call
// for a new pane always returns true.
func (t *Integration) contentLimiterAllow(key string) bool {
	t.mu.Lock()
	limiter, ok := t.contentLimiters[key]
	if !ok {
		limiter = terminal.NewRateLimiterWithInterval(contentCheckInterval)
		t.contentLimiters[key] = limiter
	}
	t.mu.Unlock()
	return limiter.Allow()
}

func (t *Integration) processFingerprint(panePID int64) int64 {
	if panePID <= 0 {
		return 0
	}
	if t.processReader == nil {
		return panePID
	}
	foregroundPID, err := t.processReader.TPGID(int(panePID))
	if err == nil && foregroundPID > 0 {
		return int64(foregroundPID)
	}
	return panePID
}

func (t *Integration) prunePaneKeysLocked(activeKeys map[string]bool) {
	for key := range t.limiters {
		if !activeKeys[key] {
			delete(t.limiters, key)
		}
	}
	for paneID := range t.contentLimiters {
		if !activeKeys[paneID] {
			delete(t.contentLimiters, paneID)
		}
	}
}

// SessionPathKey is the metadata key callers inject to pass session path.
const SessionPathKey = "_session_path"

// findSessionCache locates the sessionCache for a slug using metadata, exact match, or @hive-session tags.
// Must be called with t.mu held (read or write).
func (t *Integration) findSessionCache(slug string, metadata map[string]string) (string, *sessionCache) {
	if name := metadata[session.MetaTmuxSession]; name != "" {
		if sc, exists := t.cache[name]; exists {
			return name, sc
		}
	}
	if sc, exists := t.cache[slug]; exists {
		return slug, sc
	}
	for sessionName, sc := range t.cache {
		for _, pane := range sc.panes {
			if pane.input.HiveSession == slug {
				return sessionName, sc
			}
		}
	}
	return "", nil
}

// DiscoverSession finds a tmux session for the given slug and metadata.
func (t *Integration) DiscoverSession(_ context.Context, slug string, metadata map[string]string) (*terminal.SessionInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.cache == nil || time.Since(t.cacheTime) > 2*time.Second {
		return nil, nil
	}

	sessionName, sc := t.findSessionCache(slug, metadata)
	if sc == nil {
		return nil, nil
	}

	windowIdx := metadata[session.MetaTmuxWindow]
	if windowIdx == "" {
		windowIdx = metadata["tmux_pane"]
	}
	if pane := sc.findAgentPaneByWindow(windowIdx); pane != nil {
		return sessionInfoFromPane(sessionName, pane), nil
	}

	pane := disambiguatePane(sc, metadata[SessionPathKey], slug)
	return sessionInfoFromPane(sessionName, pane), nil
}

func disambiguatePane(sc *sessionCache, sessionPath, slug string) *cachedPane {
	if sc == nil {
		return nil
	}
	if sessionPath != "" {
		for i := range sc.panes {
			pane := &sc.panes[i]
			if pane.result.IsAgent && pane.input.WorkDir == sessionPath {
				return pane
			}
		}
	}
	if slug != "" {
		slugLower := strings.ToLower(slug)
		for i := range sc.panes {
			pane := &sc.panes[i]
			if pane.result.IsAgent && strings.Contains(strings.ToLower(pane.input.WindowName), slugLower) {
				return pane
			}
		}
	}
	return sc.bestAgentPane()
}

func sessionInfoFromPane(sessionName string, pane *cachedPane) *terminal.SessionInfo {
	if pane == nil {
		return nil
	}
	return &terminal.SessionInfo{
		Name:         sessionName,
		WindowIndex:  pane.input.WindowIndex,
		PaneID:       pane.input.PaneID,
		WindowName:   pane.input.WindowName,
		DetectedTool: pane.result.Tool,
	}
}

// GetStatus returns the current status of a specific agent pane.
func (t *Integration) GetStatus(ctx context.Context, info *terminal.SessionInfo) (terminal.Status, error) {
	if info == nil {
		return terminal.StatusMissing, nil
	}

	t.mu.Lock()
	sc, exists := t.cache[info.Name]
	if !exists {
		t.mu.Unlock()
		return terminal.StatusMissing, nil
	}

	var pane *cachedPane
	if info.PaneID != "" {
		pane = sc.findAgentPane(info.PaneID)
		if pane == nil {
			t.mu.Unlock()
			return terminal.StatusMissing, nil
		}
	} else if info.WindowIndex != "" {
		pane = sc.findAgentPaneByWindow(info.WindowIndex)
	}
	if pane == nil {
		pane = sc.bestAgentPane()
	}
	if pane == nil {
		t.mu.Unlock()
		return terminal.StatusMissing, nil
	}

	sessionName := info.Name
	paneID := pane.input.PaneID
	if pane.pollMu == nil {
		pane.pollMu = &sync.Mutex{}
	}
	pollMu := pane.pollMu
	t.mu.Unlock()

	pollMu.Lock()
	defer pollMu.Unlock()

	key := paneKey(sessionName, paneID)
	t.mu.Lock()
	sc, exists = t.cache[sessionName]
	if !exists {
		t.mu.Unlock()
		return terminal.StatusMissing, nil
	}
	pane = sc.findAgentPane(paneID)
	if pane == nil || pane.pollMu != pollMu {
		t.mu.Unlock()
		return terminal.StatusMissing, nil
	}

	prevContent := pane.state.paneContent
	activity := pane.input.Activity
	lastCaptureActive := pane.state.lastCaptureActive
	tool := pane.result.Tool
	inMode := pane.input.InMode
	paneTitle := pane.input.PaneTitle

	// The per-key limiter is the only floor on capture-pane spawn rate; it is
	// not made redundant by the activity cheap path or Observe's generation
	// idempotence. Refresh generations can arrive well under 500ms apart
	// (immediate post-action/post-load batches, a sub-500ms poll_interval),
	// each with advanced activity for a streaming pane, and generation
	// idempotence dedupes tracker transitions, not the fork/execs of
	// overlapping FetchBatch runs. A denied poll observes ≤500ms-stale
	// content, which the confirm policies absorb.
	limiter, ok := t.limiters[key]
	if !ok {
		limiter = terminal.NewRateLimiter(2)
		t.limiters[key] = limiter
	}
	generation := t.refreshGeneration
	t.mu.Unlock()

	var content string
	freshCapture := false
	switch {
	case prevContent != "" && activity == lastCaptureActive:
		content = prevContent
	case !limiter.Allow():
		content = prevContent
	default:
		var err error
		content, err = t.capture.CapturePane(ctx, paneID)
		if err != nil {
			return terminal.StatusMissing, err
		}
		freshCapture = true
		t.updatePaneState(sessionName, paneID, func(state *paneState) {
			state.paneContent = content
			state.lastCaptureActive = activity
		})
	}

	info.PaneID = paneID
	info.PaneContent = content
	info.DetectedTool = tool

	if tool == "" {
		tool = "agent"
	}

	// Every poll reaches Observe, even when content is unchanged: stability
	// itself is a debounce signal (an idle candidate must see N consecutive
	// confirming observations), and the tracker's per-generation idempotence
	// guards against the double GetStatus call per pane that
	// hive.StatusService's FetchSession/groupPaneStatuses pairing makes.
	snap := assess.Snapshot{
		Content:    content,
		Title:      paneTitle,
		Tool:       tool,
		InMode:     inMode,
		Generation: generation,
	}
	paneStatus, assessment := t.tracker.Observe(key, snap)

	if freshCapture && t.recorder != nil {
		if err := t.recorder.Record(CaptureObservation{
			SessionName: sessionName,
			PaneID:      paneID,
			Tool:        tool,
			Content:     content,
			Status:      paneStatus,
			RuleID:      assessment.RuleID,
			Signals:     assessment.Signals,
		}); err != nil {
			log.Warn().Err(err).Str("pane_id", paneID).Msg("failed to record tmux pane capture")
		}
	}

	return paneStatus, nil
}

func (t *Integration) updatePaneState(sessionName, paneID string, update func(*paneState)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if sc, ok := t.cache[sessionName]; ok {
		if pane := sc.findPane(paneID); pane != nil {
			update(&pane.state)
		}
	}
}

// DiscoverAllPanes returns a SessionInfo for every classified agent pane.
func (t *Integration) DiscoverAllPanes(_ context.Context, slug string, metadata map[string]string) ([]*terminal.SessionInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.cache == nil || time.Since(t.cacheTime) > 2*time.Second {
		return nil, nil
	}

	sessionName, sc := t.findSessionCache(slug, metadata)
	if sc == nil {
		return nil, nil
	}

	agents := sc.agentPanes()
	if len(agents) == 0 {
		return nil, nil
	}
	infos := make([]*terminal.SessionInfo, 0, len(agents))
	for i := range agents {
		infos = append(infos, sessionInfoFromPane(sessionName, &agents[i]))
	}
	return infos, nil
}

func paneKey(sessionName, paneID string) string { return sessionName + ":" + paneID }

// Ensure Integration implements terminal.Integration.
var (
	_ terminal.Integration        = (*Integration)(nil)
	_ terminal.AllPanesDiscoverer = (*Integration)(nil)
)
