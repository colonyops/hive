package doctor

import "context"

// Status represents the result status of a check item.
// ENUM(pass, warn, fail).
type Status string

// CheckItem represents a single line item within a check result.
type CheckItem struct {
	Label   string `json:"label"`
	Status  Status `json:"-"`
	Detail  string `json:"detail,omitempty"`
	Fixable bool   `json:"fixable,omitempty"`

	// For JSON output
	StatusStr string `json:"status"`
}

// Result represents the outcome of a check containing multiple items.
type Result struct {
	Name  string      `json:"name"`
	Items []CheckItem `json:"items"`
}

// Check defines the interface for a doctor check.
type Check interface {
	Name() string
	Run(ctx context.Context) Result
}

// RunAll executes all checks and returns their results.
func RunAll(ctx context.Context, checks []Check) []Result {
	results := make([]Result, 0, len(checks))
	for _, check := range checks {
		result := check.Run(ctx)
		for i := range result.Items {
			result.Items[i].StatusStr = string(result.Items[i].Status)
		}
		results = append(results, result)
	}
	return results
}

// Summary returns counts of passed, warned, and failed items across all results.
func Summary(results []Result) (passed, warned, failed int) {
	for _, r := range results {
		for _, item := range r.Items {
			switch item.Status {
			case StatusPass:
				passed++
			case StatusWarn:
				warned++
			case StatusFail:
				failed++
			}
		}
	}

	return passed, warned, failed
}

// CountFixable returns the number of fixable issues across all results.
func CountFixable(results []Result) int {
	count := 0
	for _, r := range results {
		for _, item := range r.Items {
			if item.Fixable && (item.Status == StatusWarn || item.Status == StatusFail) {
				count++
			}
		}
	}
	return count
}
