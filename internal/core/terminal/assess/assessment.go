package assess

// State is the engine's semantic classification. It is deliberately distinct
// from terminal.Status: the stateful tracker owns the mapping to published
// statuses.
type State string

const (
	StateWorking  State = "working"
	StateApproval State = "approval"
	StateQuestion State = "question"
	StateIdle     State = "idle"
	StateUnknown  State = "unknown" // no rule matched; never silently "idle"
)

// Signal records one piece of matched evidence for debugging, the capture
// recorder, and future labeling. JSON tags exist because `hive x assess file`
// serializes Signal directly as part of its diagnostic output.
type Signal struct {
	RuleID  string `json:"ruleID"`
	Region  string `json:"region"`
	Matched string `json:"matched"` // the matched text or pattern description
}

// Assessment is the engine's structured output for one Snapshot.
type Assessment struct {
	State   State
	Hold    bool // transient screen detected: keep current published state
	RuleID  string
	Signals []Signal

	// AboveBox is the engine-normalized (ANSI/NBSP) text of the
	// above-prompt-box region, handed to Stage 2 so churn hashing never
	// re-runs region extraction. Stage 2's churn normalization does re-run
	// StripANSI as boundary defense against a producer handing it
	// unstripped text; on this already-stripped field that pass is a
	// fast-path no-op.
	AboveBox string
}
