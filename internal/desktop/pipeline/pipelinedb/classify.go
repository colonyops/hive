package pipelinedb

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

const maxEventDetailBytes = 4096

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

type AbsenceConfirmer interface {
	ConfirmAbsence(context.Context, Observation) (AbsenceVerdict, error)
}

type SourceAdapter struct {
	SourceKind string
	Classifier
	AbsenceConfirmer
}

func boundEventDetail(detail []byte) []byte {
	if len(detail) <= maxEventDetailBytes {
		return detail
	}
	return append([]byte(nil), detail[:maxEventDetailBytes]...)
}
