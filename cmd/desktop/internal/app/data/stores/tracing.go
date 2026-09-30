package stores

import (
	"github.com/colonyops/hive/cmd/desktop/internal/app/observe"
)

var tracer = observe.Tracer("/internal/app/data/stores")

// Batch boundaries only, never individual statements: a per-statement span
// scaled with how much data arrived, not with configuration, and one tick
// produced spans in the hundreds
// (ADR a-span-is-a-trigger-or-a-wait-and-its-count-per-trigger-is-bounded-by-configuration).
