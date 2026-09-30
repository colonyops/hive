package models

import "context"

// Observation is an adapter-neutral representation of a source item.
type Observation struct {
	ExternalID  string
	Title       string
	URL         string
	SourceKind  string
	SourceScope string
	ObservedAt  int64
	Payload     []byte
}

// ENUM(active, terminal, unknown)
type Lifecycle string

// ENUM(none, entered-terminal, left-terminal)
type Transition string

// ENUM(activity, trivial)
type Attention string

// ENUM(manual, system)
type ArchivedActor string

// ENUM(all, state-changes, never)
type ResurfacePolicy string

// Classification is the persisted result of comparing normalized source state.
type Classification struct {
	Kind           string
	Transition     Transition
	Attention      Attention
	Lifecycle      Lifecycle
	SourceState    string
	ArchivedReason string
	OccurrenceKey  string
	Summary        string
	Detail         []byte
}

// Classifier is deliberately independent of source wire formats.
type Classifier interface {
	Classify(previous *Observation, current Observation) Classification
}

type AbsenceVerdict struct {
	Current  *Observation
	Terminal bool
}

// Verdicts are keyed by Observation.ExternalID; an item the connector could not
// answer for is absent from the map. A returned error is a partial failure —
// the verdicts already in the map are valid.
type AbsenceConfirmer interface {
	ConfirmAbsence(ctx context.Context, previous []Observation) (map[string]AbsenceVerdict, error)
}
