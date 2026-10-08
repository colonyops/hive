package actions

import (
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dirwatch"
)

func NewActionsWatcher(path string, onChange func(), logger zerolog.Logger) (*dirwatch.Watcher, error) {
	name := filepath.Base(path)
	isActionsFile := func(p string) bool { return filepath.Base(p) == name }
	return dirwatch.New(filepath.Dir(path), isActionsFile, onChange, logger, dirwatch.WithComponent("actions-watcher"))
}
