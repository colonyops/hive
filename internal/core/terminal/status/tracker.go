// Package status implements Stage 2 of the two-stage status assessment
// engine: a stateful debounce tracker that turns per-poll assess.Assessment
// values into stable, published terminal.Status values. See
// internal/core/terminal/assess for Stage 1, the stateless engine this
// package consumes.
package status

import (
	"sync"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
)

// trackedState is the per-key published/candidate state machine.
type trackedState struct {
	published         terminal.Status
	candidate         terminal.Status // pending de-escalation target, "" if none
	candidateSince    time.Time
	candidatePolls    int
	contentHash       string    // churn-normalized hash of Assessment.AboveBox
	contentHashSeeded bool      // held/copy-mode first frames deliberately leave it unset
	lastChurnAt       time.Time // when that hash last changed

	lastGeneration uint64
	lastAssessment assess.Assessment

	// candidateContentHash is the content hash recorded when the current
	// candidate started (or last restarted); see ConfirmPolicy.StableContent.
	candidateContentHash string
}

// Tracker debounces per-key assessments into published terminal.Status
// values. Observe must be called every poll cycle for every live target,
// including when content is unchanged — stability is itself a signal.
//
// A Tracker is safe for concurrent use. DebugState is the one exception:
// it exists for the single-goroutine `hive x assess watch`/`replay` tooling
// and races production GetStatus callers under concurrent use (TOCTOU) —
// see its doc comment.
type Tracker struct {
	mu      sync.Mutex
	engine  *assess.Engine
	tracked map[string]*trackedState
	opts    Options
	now     func() time.Time // injected clock; see Options.Clock
}

// NewTracker builds a Tracker around engine using opts. opts.Clock (nil =
// time.Now) is captured once here.
func NewTracker(engine *assess.Engine, opts Options) *Tracker {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Tracker{
		engine:  engine,
		tracked: make(map[string]*trackedState),
		opts:    opts,
		now:     clock,
	}
}

// Observe assesses snap and returns the published status plus the
// assessment (returned by value so there is no separate accessor to race
// against concurrent GetStatus calls). Idempotent per (key, snap.Generation):
// a repeat observation within the same refresh generation returns the
// current published status and last assessment without advancing counters
// or timers.
func (t *Tracker) Observe(key string, snap assess.Snapshot) (terminal.Status, assess.Assessment) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ts, exists := t.tracked[key]
	if exists && ts.lastGeneration == snap.Generation {
		return ts.published, ts.lastAssessment
	}

	assessment := t.engine.Assess(snap)
	now := t.now()

	if !exists {
		ts = &trackedState{published: terminal.StatusReady}
		t.tracked[key] = ts
	}

	switch {
	case snap.InMode:
		// Copy-mode hold: scrollback churn must not read as working, and a
		// scrolled viewport must not poison the next real churn comparison.
		// Published, candidate, and content-hash bookkeeping are all frozen.
	case assessment.Hold:
		// Assess-level hold (transient overlay: search prompt, transcript
		// viewer, model picker): keep published and reset the candidate, same
		// as plain unknown. But ALSO freeze content-hash/churn bookkeeping —
		// unlike plain unknown, whose churn is the intended "streaming output
		// is itself working" signal for tools with no dedicated rules. An
		// overlay's own content changes (and its close, which reverts
		// AboveBox to what it was before) must not register as churn: rule
		// authors mark it Hold precisely to keep the overlay from being
		// misread as working, and freezing the hash is what makes the
		// pre-overlay and post-overlay frames compare equal again.
		clearCandidate(ts)
	case !exists:
		t.observeFirst(ts, assessment)
	default:
		t.observeNext(ts, assessment, now)
	}

	ts.lastGeneration = snap.Generation
	ts.lastAssessment = assessment

	return ts.published, assessment
}

// observeFirst applies first-observation semantics for a brand new key after
// copy-mode and assessment holds have been excluded. Unknown publishes
// StatusReady; any definite state — including idle — publishes immediately,
// since there is no prior working state to protect yet.
func (t *Tracker) observeFirst(ts *trackedState, assessment assess.Assessment) {
	ts.published = mapDefiniteState(assessment.State)
	// Seed the churn hash without flagging churn: lastChurnAt stays zero, so
	// the very first poll never reads as a change from "nothing observed yet".
	ts.contentHash = hashContent(normalizeContent(assessment.AboveBox))
	ts.contentHashSeeded = true
}

// observeNext updates churn bookkeeping and applies the debounce transition
// table for an existing, non-copy-mode observation.
func (t *Tracker) observeNext(ts *trackedState, assessment assess.Assessment, now time.Time) {
	newHash := hashContent(normalizeContent(assessment.AboveBox))
	switch {
	case !ts.contentHashSeeded:
		ts.contentHash = newHash
		ts.contentHashSeeded = true
	case newHash != ts.contentHash:
		ts.contentHash = newHash
		ts.lastChurnAt = now
	}
	churned := !ts.lastChurnAt.IsZero() && now.Sub(ts.lastChurnAt) < t.opts.ChurnWindow

	state := assessment.State
	if churned && (state == assess.StateIdle || state == assess.StateUnknown) {
		// Output streaming is itself working, even with no recognizable
		// spinner shape — makes the generic fallback rule set usable.
		state = assess.StateWorking
	}

	t.applyTransition(ts, state, now, newHash)
}

// Reset drops all state for key so its next observation uses first-observation semantics.
func (t *Tracker) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tracked, key)
}

// Prune drops state for keys not in activeKeys.
func (t *Tracker) Prune(activeKeys map[string]bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for key := range t.tracked {
		if !activeKeys[key] {
			delete(t.tracked, key)
		}
	}
}

// DebugState is a copy of a key's tracked state for the watch/replay
// tooling, which owns a private single-goroutine Tracker. Production
// transports must not use it (TOCTOU under concurrent GetStatus).
type DebugState struct {
	Published      terminal.Status
	Candidate      terminal.Status
	CandidatePolls int
	Churned        bool // AboveBox hash changed within ChurnWindow
}

// DebugState returns the current tracked state for key, or false if key has
// never been observed (or was pruned).
func (t *Tracker) DebugState(key string) (DebugState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ts, ok := t.tracked[key]
	if !ok {
		return DebugState{}, false
	}

	now := t.now()
	churned := !ts.lastChurnAt.IsZero() && now.Sub(ts.lastChurnAt) < t.opts.ChurnWindow

	return DebugState{
		Published:      ts.published,
		Candidate:      ts.candidate,
		CandidatePolls: ts.candidatePolls,
		Churned:        churned,
	}, true
}
