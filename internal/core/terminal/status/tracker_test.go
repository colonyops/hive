package status_test

import (
	"testing"
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tiny inline content strings, each chosen (and verified against the real
// assess.Engine) to land on one specific assess.State via the generic rule
// set ("test-tool" has no dedicated rules, so it always falls back to
// genericRules):
//
//	contentWorking  -> StateWorking  (generic/spinner-shape)
//	contentApproval -> StateApproval (generic/yes-no-prompt)
//	contentQuestion -> StateQuestion (generic/question)
//	contentIdleA/B  -> StateIdle     (generic/prompt-glyph)
//	contentUnknownA -> StateUnknown  (no rule matches)
//
// contentIdleA and contentIdleB both classify as idle but render different
// AboveBox text (the line preceding the bare prompt glyph differs), so
// swapping between them changes the churn hash without changing state.
const (
	contentWorking  = "⠋ Thinking… (working)\n"
	contentApproval = "Some setup text\nContinue? (y/n)"
	contentQuestion = "Pick a color?\n1. Red\n2. Blue"
	contentIdleA    = "line one\n❯"
	contentIdleB    = "line one changed\n❯"
	contentUnknownA = "just some ordinary output with no special shape"
)

// contentIdleClaude, contentHoldOverlay, and contentWorkingClaude exercise
// assess.Assessment.Hold, which only exists in the tool-specific rule sets
// (claudeRules'/codexRules' holdRule) — the generic fallback set used by
// "test-tool" above has no hold rules at all.
const (
	contentIdleClaude    = "line one\n❯"
	contentHoldOverlay   = "some transcript overlay\nctrl+r to toggle\n"
	contentWorkingClaude = "⠋ Thinking… (working)\n"
)

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// fakeClock is the test seam for Options.Clock: advance it explicitly
// instead of relying on wall-clock time, so confirm-policy timing is
// deterministic.
type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func snap(content string, generation uint64) assess.Snapshot {
	return assess.Snapshot{Content: content, Tool: "test-tool", Generation: generation}
}

func snapClaude(content string, generation uint64) assess.Snapshot {
	return assess.Snapshot{Content: content, Tool: "claude", Generation: generation}
}

func newTracker(opts status.Options, clock *fakeClock) *status.Tracker {
	opts.Clock = clock.Now
	return status.NewTracker(assess.NewEngine(), opts)
}

// --- First observation ---

func TestTracker_FirstObservation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    terminal.Status
	}{
		{"unknown defaults to ready", contentUnknownA, terminal.StatusReady},
		{"idle publishes immediately", contentIdleA, terminal.StatusReady},
		{"working publishes immediately", contentWorking, terminal.StatusActive},
		{"approval publishes immediately", contentApproval, terminal.StatusApproval},
		{"question publishes immediately", contentQuestion, terminal.StatusQuestion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &fakeClock{t: start}
			tr := newTracker(status.DefaultOptions(), clock)

			published, _ := tr.Observe("k", snap(tt.content, 1))
			assert.Equal(t, tt.want, published)
		})
	}
}

// --- Escalation immediacy ---

func TestTracker_EscalationImmediate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    terminal.Status
	}{
		{"idle to approval", contentApproval, terminal.StatusApproval},
		{"idle to question", contentQuestion, terminal.StatusQuestion},
		{"idle to working", contentWorking, terminal.StatusActive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &fakeClock{t: start}
			tr := newTracker(status.DefaultOptions(), clock)

			published, _ := tr.Observe("k", snap(contentIdleA, 1))
			require.Equal(t, terminal.StatusReady, published)

			published, _ = tr.Observe("k", snap(tt.content, 2))
			assert.Equal(t, tt.want, published, "escalation must not wait for confirmation")
		})
	}
}

// --- working -> idle: confirmed ---

func TestTracker_WorkingToIdleConfirmed(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond // isolate idle-confirm timing from the transition's own churn event
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentWorking, 1))
	require.Equal(t, terminal.StatusActive, published)

	// The switch from working-text to idle-text is itself a churn event;
	// settle past the (deliberately short) window before counting.
	tr.Observe("k", snap(contentIdleA, 2))
	clock.Advance(100 * time.Millisecond)

	published, _ = tr.Observe("k", snap(contentIdleA, 3))
	assert.Equal(t, terminal.StatusActive, published, "1 poll: not yet confirmed")

	clock.Advance(1 * time.Second)
	published, _ = tr.Observe("k", snap(contentIdleA, 4))
	assert.Equal(t, terminal.StatusActive, published, "polls satisfied but duration (1s of 2s) is not")

	clock.Advance(1500 * time.Millisecond)
	published, _ = tr.Observe("k", snap(contentIdleA, 5))
	assert.Equal(t, terminal.StatusReady, published, "polls and duration both satisfied")
}

// --- Stability vs. elapsed time ---

func TestTracker_IdleConfirmStabilityVsElapsed(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond // keep the coarse override from masking the per-candidate check
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentWorking, 1))
	require.Equal(t, terminal.StatusActive, published)

	gen := uint64(2)
	observe := func(content string, advance time.Duration) terminal.Status {
		clock.Advance(advance)
		got, _ := tr.Observe("k", snap(content, gen))
		gen++
		return got
	}

	// Settle the transition's own churn event.
	require.Equal(t, terminal.StatusActive, observe(contentIdleA, 20*time.Millisecond))

	// Content changes every poll: even though 5 polls and >5s elapse, the
	// per-candidate stability check never lets it commit.
	for i := 0; i < 5; i++ {
		content := contentIdleA
		if i%2 == 1 {
			content = contentIdleB
		}
		got := observe(content, 1200*time.Millisecond)
		assert.Equal(t, terminal.StatusActive, got, "content still churning at step %d", i)
	}

	// Stabilizes: same content, 2 polls, >=2s apart.
	require.Equal(t, terminal.StatusActive, observe(contentIdleA, 1200*time.Millisecond))
	got := observe(contentIdleA, 2*time.Second)
	assert.Equal(t, terminal.StatusReady, got, "both polls and duration satisfied once content stabilizes")
}

// --- approval -> anything: confirmed ---

func TestTracker_ApprovalExitSingleConfirm(t *testing.T) {
	clock := &fakeClock{t: start}
	tr := newTracker(status.DefaultOptions(), clock) // ConfirmApproval{Polls: 1}

	published, _ := tr.Observe("k", snap(contentApproval, 1))
	require.Equal(t, terminal.StatusApproval, published)

	// A single poll showing the dialog is gone is enough — contrast with
	// the idle-confirm path's 2 polls / 2s (TestTracker_WorkingToIdleConfirmed).
	published, _ = tr.Observe("k", snap(contentWorking, 2))
	assert.Equal(t, terminal.StatusActive, published)
}

func TestTracker_ApprovalExitRespectsConfiguredPolls(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ConfirmApproval = status.ConfirmPolicy{Polls: 2}
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentApproval, 1))
	require.Equal(t, terminal.StatusApproval, published)

	published, _ = tr.Observe("k", snap(contentWorking, 2))
	assert.Equal(t, terminal.StatusApproval, published, "1st confirming poll: not yet enough")

	published, _ = tr.Observe("k", snap(contentWorking, 3))
	assert.Equal(t, terminal.StatusActive, published, "2nd confirming poll satisfies the configured Polls:2")
}

// --- approval <-> question: immediate within the tier ---

func TestTracker_ApprovalQuestionSwitchImmediate(t *testing.T) {
	clock := &fakeClock{t: start}
	tr := newTracker(status.DefaultOptions(), clock)

	published, _ := tr.Observe("k", snap(contentApproval, 1))
	require.Equal(t, terminal.StatusApproval, published)

	published, _ = tr.Observe("k", snap(contentQuestion, 2))
	assert.Equal(t, terminal.StatusQuestion, published, "switching within the approval tier is immediate")
}

// --- unknown / hold ---

func TestTracker_UnknownHoldsPublished(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentWorking, 1))
	require.Equal(t, terminal.StatusActive, published)

	// Settle the transition's own churn event before asserting the hold.
	tr.Observe("k", snap(contentUnknownA, 2))
	clock.Advance(100 * time.Millisecond)

	published, _ = tr.Observe("k", snap(contentUnknownA, 3))
	assert.Equal(t, terminal.StatusActive, published, "unknown holds the last published status")

	debug, ok := tr.DebugState("k")
	require.True(t, ok)
	assert.Empty(t, debug.Candidate, "unknown resets any pending candidate")
	assert.Zero(t, debug.CandidatePolls)
}

// --- Assess-level hold (transient overlays) ---
//
// These are distinct from TestTracker_UnknownHoldsPublished above: that test
// covers plain StateUnknown with Hold=false (no rule matched at all — the
// intended churn-as-working path for tools with no dedicated rules).
// assessment.Hold marks a *recognized* transient overlay (search prompt,
// transcript viewer, model picker) and must additionally freeze churn
// bookkeeping — the overlay's own content, and the reversion when it closes,
// must never register as churn.

func TestTracker_AssessHoldDoesNotFlashWorking(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 200 * time.Millisecond // generous: prove the freeze, not a lucky window miss
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snapClaude(contentIdleClaude, 1))
	require.Equal(t, terminal.StatusReady, published)

	published, assessment := tr.Observe("k", snapClaude(contentHoldOverlay, 2))
	require.True(t, assessment.Hold, "fixture must actually trigger a hold rule")
	assert.Equal(t, terminal.StatusReady, published, "hold must not flash working even though AboveBox changed")

	debug, ok := tr.DebugState("k")
	require.True(t, ok)
	assert.Empty(t, debug.Candidate)
	assert.False(t, debug.Churned, "hold freezes churn bookkeeping too")
}

func TestTracker_AssessHoldFreezesChurnAcrossOverlayCycle(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 200 * time.Millisecond
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snapClaude(contentIdleClaude, 1))
	require.Equal(t, terminal.StatusReady, published)

	clock.Advance(50 * time.Millisecond)
	published, assessment := tr.Observe("k", snapClaude(contentHoldOverlay, 2))
	require.True(t, assessment.Hold)
	require.Equal(t, terminal.StatusReady, published)

	// Overlay closes: content reverts to exactly what it was before. This
	// only reads as "no churn" if the hold frame froze contentHash/lastChurnAt
	// instead of recording the overlay's own (different) AboveBox.
	clock.Advance(50 * time.Millisecond)
	published, _ = tr.Observe("k", snapClaude(contentIdleClaude, 3))
	assert.Equal(t, terminal.StatusReady, published, "back to the pre-overlay content: still ready throughout")

	debug, ok := tr.DebugState("k")
	require.True(t, ok)
	assert.Empty(t, debug.Candidate)
	assert.False(t, debug.Churned, "returning to identical pre-overlay content must not read as churn")
}

func TestTracker_AssessHoldResetsInProgressIdleCandidate(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snapClaude(contentWorkingClaude, 1))
	require.Equal(t, terminal.StatusActive, published)

	// Settle the transition's own churn event, then take 1 real idle poll —
	// starts a candidate but doesn't confirm.
	tr.Observe("k", snapClaude(contentIdleClaude, 2))
	clock.Advance(100 * time.Millisecond)
	tr.Observe("k", snapClaude(contentIdleClaude, 3))

	debug, ok := tr.DebugState("k")
	require.True(t, ok)
	require.Equal(t, 1, debug.CandidatePolls, "one idle poll recorded")

	// A hold overlay interrupts: candidate is cleared outright, published
	// stays active (the overlay carries no evidence either way).
	clock.Advance(50 * time.Millisecond)
	published, assessment := tr.Observe("k", snapClaude(contentHoldOverlay, 4))
	require.True(t, assessment.Hold)
	assert.Equal(t, terminal.StatusActive, published)

	debug, ok = tr.DebugState("k")
	require.True(t, ok)
	assert.Empty(t, debug.Candidate)
	assert.Zero(t, debug.CandidatePolls)

	// Overlay closes: churn bookkeeping was frozen during the hold, so this
	// identical-to-pre-overlay idle frame doesn't churn either — confirmation
	// restarts cleanly at 1, not blocked by spurious churn from the overlay.
	clock.Advance(50 * time.Millisecond)
	tr.Observe("k", snapClaude(contentIdleClaude, 5))

	debug, ok = tr.DebugState("k")
	require.True(t, ok)
	assert.Equal(t, 1, debug.CandidatePolls, "confirmation restarts from scratch after the hold")
}

// --- Generation idempotence ---

func TestTracker_GenerationIdempotence(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentWorking, 1))
	require.Equal(t, terminal.StatusActive, published)

	tr.Observe("k", snap(contentIdleA, 2))
	clock.Advance(100 * time.Millisecond)

	published1, assessment1 := tr.Observe("k", snap(contentIdleA, 3))
	debugAfterFirst, ok := tr.DebugState("k")
	require.True(t, ok)
	require.Equal(t, 1, debugAfterFirst.CandidatePolls)

	// Repeat: same key, same generation. Must return identical results and
	// must not advance any counters.
	published2, assessment2 := tr.Observe("k", snap(contentIdleA, 3))
	debugAfterRepeat, ok := tr.DebugState("k")
	require.True(t, ok)

	assert.Equal(t, published1, published2)
	assert.Equal(t, assessment1, assessment2)
	assert.Equal(t, debugAfterFirst.CandidatePolls, debugAfterRepeat.CandidatePolls, "repeat generation must not advance the candidate counter")

	// New generation: advances again.
	clock.Advance(10 * time.Millisecond)
	tr.Observe("k", snap(contentIdleA, 4))
	debugAfterNext, ok := tr.DebugState("k")
	require.True(t, ok)
	assert.Greater(t, debugAfterNext.CandidatePolls, debugAfterRepeat.CandidatePolls, "a new generation must advance")
}

// --- Flap ---

func TestTracker_FlapCancelsCandidate(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentWorking, 1))
	require.Equal(t, terminal.StatusActive, published)

	tr.Observe("k", snap(contentIdleA, 2))
	clock.Advance(100 * time.Millisecond)
	tr.Observe("k", snap(contentIdleA, 3))

	debug, ok := tr.DebugState("k")
	require.True(t, ok)
	require.Equal(t, 1, debug.CandidatePolls, "one idle poll recorded")

	// Back to working: cancels the idle candidate outright.
	clock.Advance(100 * time.Millisecond)
	published, _ = tr.Observe("k", snap(contentWorking, 4))
	require.Equal(t, terminal.StatusActive, published)

	debug, ok = tr.DebugState("k")
	require.True(t, ok)
	assert.Empty(t, debug.Candidate, "candidate cleared, not merely paused")
	assert.Zero(t, debug.CandidatePolls)

	// A fresh idle poll starts over at 1, discarding the earlier progress.
	tr.Observe("k", snap(contentIdleA, 5))
	clock.Advance(100 * time.Millisecond)
	tr.Observe("k", snap(contentIdleA, 6))

	debug, ok = tr.DebugState("k")
	require.True(t, ok)
	assert.Equal(t, 1, debug.CandidatePolls, "fresh candidate starts at 1")
}

// --- Churn as working ---

func TestTracker_ChurnAsWorking(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 200 * time.Millisecond
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentIdleA, 1))
	require.Equal(t, terminal.StatusReady, published)

	published, _ = tr.Observe("k", snap(contentIdleB, 2))
	assert.Equal(t, terminal.StatusActive, published, "churn overrides idle to working, even with no spinner shape")

	clock.Advance(50 * time.Millisecond)
	published, _ = tr.Observe("k", snap(contentIdleB, 3))
	assert.Equal(t, terminal.StatusActive, published, "override persists inside the churn window")

	clock.Advance(500 * time.Millisecond)
	published, _ = tr.Observe("k", snap(contentIdleB, 4))
	assert.Equal(t, terminal.StatusActive, published, "idle candidate just starting, not yet confirmed")

	debug, ok := tr.DebugState("k")
	require.True(t, ok)
	assert.False(t, debug.Churned, "churn window has expired")
	assert.Equal(t, terminal.StatusReady, debug.Candidate)
}

func TestTracker_EmptyAboveBoxNeverChurns(t *testing.T) {
	clock := &fakeClock{t: start}
	tr := newTracker(status.DefaultOptions(), clock)

	published, _ := tr.Observe("k", snap("", 1))
	require.Equal(t, terminal.StatusReady, published)

	for i := uint64(2); i < 6; i++ {
		clock.Advance(10 * time.Millisecond)
		published, _ = tr.Observe("k", snap("", i))
		assert.Equal(t, terminal.StatusReady, published)

		debug, ok := tr.DebugState("k")
		require.True(t, ok)
		assert.False(t, debug.Churned, "empty AboveBox is stable, never churn")
	}
}

// --- Copy-mode hold ---

func TestTracker_CopyModeHoldsPublishedAndCandidate(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond
	tr := newTracker(opts, clock)

	published, _ := tr.Observe("k", snap(contentWorking, 1))
	require.Equal(t, terminal.StatusActive, published)

	tr.Observe("k", snap(contentIdleA, 2))
	clock.Advance(100 * time.Millisecond)
	tr.Observe("k", snap(contentIdleA, 3))
	debugBefore, ok := tr.DebugState("k")
	require.True(t, ok)
	require.Equal(t, 1, debugBefore.CandidatePolls)

	scrolled := assess.Snapshot{
		Content:    "some entirely different scrollback content",
		Tool:       "test-tool",
		InMode:     true,
		Generation: 4,
	}
	published, _ = tr.Observe("k", scrolled)
	assert.Equal(t, terminal.StatusActive, published, "copy-mode frames must not change published status")

	debugAfter, ok := tr.DebugState("k")
	require.True(t, ok)
	assert.Equal(t, debugBefore, debugAfter, "copy-mode must not alter tracked state at all")
}

// --- Prune ---

func TestTracker_Prune(t *testing.T) {
	clock := &fakeClock{t: start}
	opts := status.DefaultOptions()
	opts.ChurnWindow = 10 * time.Millisecond
	tr := newTracker(opts, clock)

	tr.Observe("keep", snap(contentWorking, 1))
	tr.Observe("drop", snap(contentWorking, 1))

	tr.Observe("keep", snap(contentIdleA, 2))
	clock.Advance(100 * time.Millisecond)
	tr.Observe("keep", snap(contentIdleA, 3))
	before, ok := tr.DebugState("keep")
	require.True(t, ok)
	require.Equal(t, 1, before.CandidatePolls)

	tr.Prune(map[string]bool{"keep": true})

	_, ok = tr.DebugState("drop")
	assert.False(t, ok, "pruned key has no tracked state")

	after, ok := tr.DebugState("keep")
	require.True(t, ok)
	assert.Equal(t, before, after, "survivor keeps its candidate state across Prune")

	published, _ := tr.Observe("drop", snap(contentIdleA, 1))
	assert.Equal(t, terminal.StatusReady, published, "pruned key starts fresh (first-observation semantics apply again)")
}
