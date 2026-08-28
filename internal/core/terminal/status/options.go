package status

import (
	"math"
	"time"

	"github.com/colonyops/hive/internal/core/config"
)

// defaultChurnWindow is how recently Assessment.AboveBox must have changed
// for the churn-as-working override to apply (see Tracker's observeNext).
const defaultChurnWindow = 3 * time.Second

// defaultPollInterval mirrors config.TmuxConfig's own default so a Tracker
// built without config bridging still folds MinDuration into a poll-count
// floor sanely.
const defaultPollInterval = 1500 * time.Millisecond

// ConfirmPolicy expresses one confirmation requirement in both consecutive
// polls and wall-clock time. Hive samples (~1.5s default) rather than
// streaming, so a poll-only count is fooled by irregular scheduling and a
// duration-only bound is fooled by fast polling — both constraints must
// hold: effective polls = max(Polls, ceil(MinDuration/pollInterval)).
//
// StableContent additionally requires the churn-normalized content hash to
// stay unchanged for the entire candidacy: any change restarts the
// confirmation clock. This is stricter than (and independent of) the
// tracker's ChurnWindow-based override — see Tracker.observeNext — which
// only reacts to changes within the last ChurnWindow.
type ConfirmPolicy struct {
	Polls         int
	MinDuration   time.Duration
	StableContent bool
}

// Options configures Tracker debounce behavior.
type Options struct {
	ConfirmIdle     ConfirmPolicy // leaving working for idle
	ConfirmApproval ConfirmPolicy // leaving approval/question for anything else; entering is always immediate
	ChurnWindow     time.Duration
	PollInterval    time.Duration // transport cadence, for duration->polls conversion

	// Clock returns the current time. nil uses time.Now. This is the
	// deliberate seam for `hive x assess replay` (and in-package tests) to
	// drive the tracker with a virtual or scripted clock instead of wall
	// time.
	Clock func() time.Time
}

// DefaultOptions returns the shipped debounce configuration.
func DefaultOptions() Options {
	return Options{
		ConfirmIdle: ConfirmPolicy{
			Polls:         2,
			MinDuration:   2 * time.Second,
			StableContent: true,
		},
		ConfirmApproval: ConfirmPolicy{
			Polls: 1,
		},
		ChurnWindow:  defaultChurnWindow,
		PollInterval: defaultPollInterval,
	}
}

// effectivePolls folds a ConfirmPolicy's wall-clock requirement into a poll
// count using this Options' PollInterval, and floors the result at 1 poll.
func (o Options) effectivePolls(p ConfirmPolicy) int {
	polls := p.Polls
	if o.PollInterval > 0 && p.MinDuration > 0 {
		durationPolls := int(math.Ceil(float64(p.MinDuration) / float64(o.PollInterval)))
		if durationPolls > polls {
			polls = durationPolls
		}
	}
	if polls < 1 {
		polls = 1
	}
	return polls
}

// OptionsFromConfig bridges the terminal: config section into Tracker
// Options. It lives in this package (rather than config importing status)
// because internal/core/config is meant to stay a dependency-light,
// data-only package that many other packages import; status already sits
// under internal/core/terminal and is naturally the consumer here.
//
// Missing's policy is deliberately not part of Options: the tmux
// transport consumes config.TerminalConfirmConfig.Missing directly to decide
// how many consecutive list-panes failures to tolerate before publishing
// StatusMissing — that's a transport-level retry count, not a Tracker
// debounce rule.
func OptionsFromConfig(cfg config.TerminalStatusConfig, pollInterval time.Duration) Options {
	opts := DefaultOptions()
	opts.PollInterval = pollInterval
	opts.ConfirmIdle = confirmPolicyFromConfig(cfg.Confirm.Idle)
	opts.ConfirmApproval = confirmPolicyFromConfig(cfg.Confirm.Approval)
	return opts
}

func confirmPolicyFromConfig(p config.ConfirmPolicyConfig) ConfirmPolicy {
	return ConfirmPolicy{
		Polls:         p.Polls,
		MinDuration:   p.MinDuration,
		StableContent: p.StableContent != nil && *p.StableContent,
	}
}
