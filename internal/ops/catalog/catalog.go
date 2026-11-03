// Package catalog lists every operation of every toolset. Both programs walk
// it: the desktop to check what it mounts, the CLI to build its commands.
package catalog

import (
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/layout"
)

func All() []ops.Op {
	return layout.All()
}
