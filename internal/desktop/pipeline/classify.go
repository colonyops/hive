package pipeline

import "github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"

// Re-export the adapter-neutral contract at the producer boundary. The leaf
// pipelinedb package owns it because ingestion invokes classifiers in-tx.
type (
	Observation      = pipelinedb.Observation
	Lifecycle        = pipelinedb.Lifecycle
	Transition       = pipelinedb.Transition
	Attention        = pipelinedb.Attention
	ArchivedActor    = pipelinedb.ArchivedActor
	Classification   = pipelinedb.Classification
	Classifier       = pipelinedb.Classifier
	AbsenceVerdict   = pipelinedb.AbsenceVerdict
	AbsenceConfirmer = pipelinedb.AbsenceConfirmer
	SourceAdapter    = pipelinedb.SourceAdapter
)
