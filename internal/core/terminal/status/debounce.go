package status

import (
	"time"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
)

// applyTransition implements the debounce transition table for an existing
// key (state has already had the churn-as-working override applied):
//
//	→ approval / question   immediate (presence-based; user blocked)
//	idle → working          immediate (false positive = brief flash)
//	working → idle          confirmed via ConfirmIdle
//	approval → anything     confirmed via ConfirmApproval (dialog gone = user acted)
//	unknown                 hold: keep published, reset candidate
//
// The missing row is transport-level (see the tmux integration's
// handleRefreshFailure) and out of scope here.
func (t *Tracker) applyTransition(ts *trackedState, state assess.State, now time.Time, contentHash string) {
	if state == assess.StateUnknown {
		// No evidence must not advance an idle candidate.
		clearCandidate(ts)
		return
	}

	desired := mapDefiniteState(state)

	if isApprovalTier(ts.published) {
		if isApprovalTier(desired) {
			// Switching between approval and question, or reaffirming the
			// same one: still blocked on the user, immediate either way.
			ts.published = desired
			clearCandidate(ts)
			return
		}
		t.confirmTransition(ts, desired, t.opts.ConfirmApproval, now, contentHash)
		return
	}

	switch desired {
	case terminal.StatusApproval, terminal.StatusQuestion, terminal.StatusActive:
		ts.published = desired
		clearCandidate(ts)
	case terminal.StatusReady:
		if ts.published == terminal.StatusReady {
			clearCandidate(ts)
			return
		}
		t.confirmTransition(ts, terminal.StatusReady, t.opts.ConfirmIdle, now, contentHash)
	default:
		// mapDefiniteState never returns StatusMissing; defensive no-op
		// keeps this switch honest for the exhaustive lint gate.
	}
}

// confirmTransition advances (or starts) a candidate confirmation toward
// desired, publishing once policy's poll count, wall-clock duration, and
// (if required) content stability all hold.
func (t *Tracker) confirmTransition(ts *trackedState, desired terminal.Status, policy ConfirmPolicy, now time.Time, contentHash string) {
	switch {
	case ts.candidate != desired:
		ts.candidate = desired
		ts.candidateSince = now
		ts.candidatePolls = 1
		ts.candidateContentHash = contentHash
	case policy.StableContent && contentHash != ts.candidateContentHash:
		// Content moved during the candidacy: restart the clock so a
		// genuinely stable window must follow the change.
		ts.candidateSince = now
		ts.candidatePolls = 1
		ts.candidateContentHash = contentHash
	default:
		ts.candidatePolls++
	}

	if ts.candidatePolls < t.opts.effectivePolls(policy) {
		return
	}
	if policy.MinDuration > 0 && now.Sub(ts.candidateSince) < policy.MinDuration {
		return
	}

	ts.published = desired
	clearCandidate(ts)
}

func clearCandidate(ts *trackedState) {
	ts.candidate = ""
	ts.candidateSince = time.Time{}
	ts.candidatePolls = 0
	ts.candidateContentHash = ""
}

func isApprovalTier(s terminal.Status) bool {
	return s == terminal.StatusApproval || s == terminal.StatusQuestion
}

// mapDefiniteState maps a non-unknown assess.State to the terminal.Status the
// tracker publishes for it. Unknown is handled by callers before mapping,
// except in first-observation, where it falls through to StatusReady.
func mapDefiniteState(state assess.State) terminal.Status {
	switch state {
	case assess.StateWorking:
		return terminal.StatusActive
	case assess.StateApproval:
		return terminal.StatusApproval
	case assess.StateQuestion:
		return terminal.StatusQuestion
	case assess.StateIdle:
		return terminal.StatusReady
	default: // assess.StateUnknown
		return terminal.StatusReady
	}
}
