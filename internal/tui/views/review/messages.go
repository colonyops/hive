package review

import "github.com/colonyops/hive/internal/core/action"

// ActionRequestMsg requests the parent to execute a resolved action.
type ActionRequestMsg struct {
	Action action.Action
}
