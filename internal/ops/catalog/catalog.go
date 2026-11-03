// Package catalog lists every operation of every toolset. Both programs walk
// it: the desktop to check what it mounts, the CLI to build its commands.
package catalog

import (
	"slices"

	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/actions"
	"github.com/colonyops/hive/internal/ops/diagnostics"
	"github.com/colonyops/hive/internal/ops/drive"
	"github.com/colonyops/hive/internal/ops/inbox"
	"github.com/colonyops/hive/internal/ops/layout"
	"github.com/colonyops/hive/internal/ops/sessions"
)

func All() []ops.Op {
	return slices.Concat(
		layout.All(), drive.All(),
		inbox.All(), sessions.All(), actions.All(), diagnostics.All(),
	)
}
