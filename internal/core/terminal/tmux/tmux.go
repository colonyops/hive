// Package tmux implements terminal integration for tmux.
package tmux

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/colonyops/hive/internal/core/terminal/process"
)

// Integration implements terminal.Integration for tmux.
type Integration struct {
	mu            sync.RWMutex
	cache         map[string]*sessionCache
	cacheTime     time.Time
	trackers      map[string]*terminal.StateTracker
	limiters      map[string]*terminal.RateLimiter
	available     bool
	availableOnce sync.Once
	classifier    *classifier.Classifier
	classCache    *classifier.Cache
	lister        PaneLister
	capture       classifier.ContentCapture
}

// sessionCache holds all panes for a single tmux session.
type sessionCache struct {
	panes []cachedPane
}

// cachedPane combines pane identity, classification output, and polling state.
type cachedPane struct {
	input  classifier.PaneInput
	result classifier.Result
	state  paneState
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

// New creates a new tmux integration.
func New(cls *classifier.Classifier, lister PaneLister) *Integration {
	capture := TmuxCapture{}
	if cls == nil {
		cls = classifier.New(nil, process.OSReader{}, capture, nil)
	}
	if lister == nil {
		lister = TmuxPaneLister{}
	}
	return &Integration{
		cache:      make(map[string]*sessionCache),
		trackers:   make(map[string]*terminal.StateTracker),
		limiters:   make(map[string]*terminal.RateLimiter),
		classifier: cls,
		classCache: classifier.NewCache(),
		lister:     lister,
		capture:    capture,
	}
}

// Name returns "tmux".
func (t *Integration) Name() string { return "tmux" }

// Available returns true if tmux is installed and accessible.
func (t *Integration) Available() bool {
	t.availableOnce.Do(func() {
		_, err := exec.LookPath("tmux")
		t.available = err == nil
	})
	return t.available
}

// RefreshCache updates cached pane classifications. Call once per poll cycle.
func (t *Integration) RefreshCache() {
	panes, err := t.lister.ListAllPanes()
	if err != nil {
		log.Debug().Err(err).Msg("tmux list-panes failed, clearing cache")
		t.mu.Lock()
		t.cache = make(map[string]*sessionCache)
		t.cacheTime = time.Time{}
		t.prunePaneKeysLocked(map[string]bool{})
		t.mu.Unlock()
		return
	}

	t.mu.RLock()
	type paneSnapshot struct {
		pid   int64
		state paneState
	}
	oldStates := make(map[string]paneSnapshot)
	for sessionName, sc := range t.cache {
		for _, pane := range sc.panes {
			oldStates[paneKey(sessionName, pane.input.PaneID)] = paneSnapshot{pid: pane.input.PanePID, state: pane.state}
		}
	}
	t.mu.RUnlock()

	newCache := make(map[string]*sessionCache)
	activePaneIDs := make(map[string]bool, len(panes))
	activeKeys := make(map[string]bool, len(panes))
	for _, input := range panes {
		if input.SessionName == "" || input.PaneID == "" {
			continue
		}
		activePaneIDs[input.PaneID] = true
		key := paneKey(input.SessionName, input.PaneID)
		activeKeys[key] = true

		result, ok := t.classCache.Get(input.PaneID, input.PanePID)
		if !ok {
			result = t.classifier.Classify(context.Background(), input)
			t.classCache.Set(input.PaneID, input.PanePID, result)
		}

		var state paneState
		if snapshot, ok := oldStates[key]; ok && snapshot.pid == input.PanePID {
			state = snapshot.state
		}
		entry := cachedPane{input: input, result: result, state: state}
		sc := newCache[input.SessionName]
		if sc == nil {
			sc = &sessionCache{}
			newCache[input.SessionName] = sc
		}
		sc.panes = append(sc.panes, entry)
	}

	t.classCache.Prune(activePaneIDs)
	t.mu.Lock()
	t.cache = newCache
	t.cacheTime = time.Now()
	t.prunePaneKeysLocked(activeKeys)
	t.mu.Unlock()
}

func (t *Integration) prunePaneKeysLocked(activeKeys map[string]bool) {
	for key := range t.trackers {
		if !activeKeys[key] {
			delete(t.trackers, key)
		}
	}
	for key := range t.limiters {
		if !activeKeys[key] {
			delete(t.limiters, key)
		}
	}
}

// SessionPathKey is the metadata key callers inject to pass session path.
const SessionPathKey = "_session_path"

// findSessionCache locates the sessionCache for a slug using metadata or exact match.
func (t *Integration) findSessionCache(slug string, metadata map[string]string) (string, *sessionCache) {
	if name := metadata[session.MetaTmuxSession]; name != "" {
		if sc, exists := t.cache[name]; exists {
			return name, sc
		}
	}
	if sc, exists := t.cache[slug]; exists {
		return slug, sc
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

	t.mu.RLock()
	sc, exists := t.cache[info.Name]
	if !exists {
		t.mu.RUnlock()
		return terminal.StatusMissing, nil
	}

	var pane *cachedPane
	if info.PaneID != "" {
		pane = sc.findAgentPane(info.PaneID)
		if pane == nil {
			t.mu.RUnlock()
			return terminal.StatusMissing, nil
		}
	} else if info.WindowIndex != "" {
		pane = sc.findAgentPaneByWindow(info.WindowIndex)
	}
	if pane == nil {
		pane = sc.bestAgentPane()
	}
	if pane == nil {
		t.mu.RUnlock()
		return terminal.StatusMissing, nil
	}

	sessionName := info.Name
	paneID := pane.input.PaneID
	key := paneKey(sessionName, paneID)
	prevContent := pane.state.paneContent
	activity := pane.input.Activity
	lastCaptureActive := pane.state.lastCaptureActive
	cachedStatus := pane.state.cachedStatus
	tool := pane.result.Tool
	t.mu.RUnlock()

	t.mu.Lock()
	limiter, ok := t.limiters[key]
	if !ok {
		limiter = terminal.NewRateLimiter(2)
		t.limiters[key] = limiter
	}
	t.mu.Unlock()

	var content string
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
		t.updatePaneState(sessionName, paneID, func(state *paneState) {
			state.paneContent = content
			state.lastCaptureActive = activity
		})
	}

	info.PaneID = paneID
	info.PaneContent = content
	info.DetectedTool = tool

	if content == prevContent && cachedStatus != "" {
		return cachedStatus, nil
	}

	t.mu.Lock()
	tracker, ok := t.trackers[key]
	if !ok {
		tracker = terminal.NewStateTracker()
		t.trackers[key] = tracker
	}
	t.mu.Unlock()

	if tool == "" {
		tool = "agent"
	}
	status := tracker.Update(content, activity, terminal.NewDetector(tool))
	t.updatePaneState(sessionName, paneID, func(state *paneState) { state.cachedStatus = status })

	return status, nil
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

// DiscoverAllWindows delegates to DiscoverAllPanes for backward compatibility.
func (t *Integration) DiscoverAllWindows(ctx context.Context, slug string, metadata map[string]string) ([]*terminal.SessionInfo, error) {
	return t.DiscoverAllPanes(ctx, slug, metadata)
}

func paneKey(sessionName, paneID string) string { return sessionName + ":" + paneID }

// Ensure Integration implements terminal.Integration.
var (
	_ terminal.Integration          = (*Integration)(nil)
	_ terminal.AllPanesDiscoverer   = (*Integration)(nil)
	_ terminal.AllWindowsDiscoverer = (*Integration)(nil)
)
