package flow

import (
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dirwatch"
)

func NewFlowsWatcher(dir string, onChange func(), logger zerolog.Logger) (*dirwatch.Watcher, error) {
	return dirwatch.New(dir, isFlowFile, onChange, logger, dirwatch.WithComponent("flows-watcher"))
}

// isFlowFile excludes .sidebar.yaml: the frontend owns that state and applies
// it optimistically, so a reload on its writes would flash the sidebar.
func isFlowFile(name string) bool {
	base := filepath.Base(name)
	if strings.HasSuffix(base, ".sidebar.yaml") || strings.HasSuffix(base, ".sidebar.yml") {
		return false
	}
	ext := filepath.Ext(base)
	return ext == ".yaml" || ext == ".yml"
}
