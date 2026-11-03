package ops

// The two ends of the surface-wide detail ladder.
//
// Every operation whose answer carries source-supplied JSON (an inbox item's
// payload, an event's detail, a dry run's messages) answers at a size set by
// data nobody here chose. Rather than a cap that silently truncates, each one
// takes a detail argument and reports the level it answered at, so an omitted
// field is never mistaken for an absent one. An operation may add rungs
// between these; summary and full mean the same thing everywhere.
const (
	// DetailSummary omits every source-supplied payload.
	DetailSummary = "summary"
	// DetailFull holds nothing back.
	DetailFull = "full"
)

// SummaryOrFull is the detail constraint of an operation with no middle rung.
var SummaryOrFull = Constraint{Enum: []string{DetailSummary, DetailFull}, Default: DetailSummary}

// NoInput is the input of an operation that takes no arguments.
type NoInput struct{}
