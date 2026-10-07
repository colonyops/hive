package flow

import (
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dirwatch"
)

// NewFlowsWatcher invokes onChange when a flow file in dir changes on disk,
// so edits made outside the app, and the app's own SaveFlow/SaveLayout
// writes, apply live.
func NewFlowsWatcher(dir string, onChange func(), logger zerolog.Logger) (*dirwatch.Watcher, error) {
	return dirwatch.New(dir, isFlowFile, onChange, logger, dirwatch.WithComponent("flows-watcher"))
}

// isFlowFile reports whether name (as delivered by fsnotify — a path inside
// the watched directory) is worth a reload. It matches *.yaml/*.yml — flow
// definitions and their sibling .ui.yaml layouts, where a layout-only edit
// triggers the same (cheap, idempotent) Reload as a flow edit — but excludes
// .sidebar.yaml files. The sidebar layout (feed folders + order) is per-profile
// UI state the frontend owns and applies optimistically; reloading + emitting
// flows:updated on its writes would make the frontend blank and refetch the
// sidebar, causing a visible flash on every folder toggle or reorder.
func isFlowFile(name string) bool {
	base := filepath.Base(name)
	if strings.HasSuffix(base, ".sidebar.yaml") || strings.HasSuffix(base, ".sidebar.yml") {
		return false
	}
	ext := filepath.Ext(base)
	return ext == ".yaml" || ext == ".yml"
}
