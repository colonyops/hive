package classifier

import "time"

// Confidence represents classification certainty.
type Confidence string

const (
	// ConfidenceHigh indicates a strong classification signal.
	ConfidenceHigh Confidence = "high"
	// ConfidenceMedium indicates a moderate classification signal.
	ConfidenceMedium Confidence = "medium"
)

// Result holds the classification output for a single pane.
type Result struct {
	IsAgent      bool
	Tool         string
	Confidence   Confidence
	Tier         int
	ClassifiedAt time.Time
}

// StableForProcessCache reports whether this positive result can be reused
// while the pane foreground process fingerprint is unchanged.
func (r Result) StableForProcessCache() bool {
	return r.IsAgent && r.Tier != tierContent
}
