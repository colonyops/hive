package app

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app/actions"
)

// TestOutputExecutorsCoverEveryActionType is the dispatch side of the
// executor-map bijection: every action type actions.Types() returns (the
// registered actions.yml catalog) must have an Executor in the map the
// output worker dispatches through, or a configured action of that type
// would silently go unexecuted.
//
// The reverse is not asserted: the map also carries dispatch.ActionTypeNotify,
// the flow notify terminal's own action type, which has no actions.Types()
// entry — see outputExecutors' doc comment. Coverage of the catalog is the
// invariant here, not set equality with the map's keys.
func TestOutputExecutorsCoverEveryActionType(t *testing.T) {
	executors := outputExecutors(nil, nil, nil, nil, nil, nil, nil, zerolog.Nop())

	for _, actionType := range actions.Types() {
		_, ok := executors[actionType]
		assert.Truef(t, ok, "action type %q (from actions.Types()) has no dispatch executor", actionType)
	}
}
