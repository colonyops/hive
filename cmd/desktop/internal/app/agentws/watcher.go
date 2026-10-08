package agentws

import (
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dirwatch"
)

// NewWatcher deliberately stops one level down, so agent writes into docs/ and
// the generator's own output never trigger a reload.
func NewWatcher(root string, onChange func(), logger zerolog.Logger) (*dirwatch.Watcher, error) {
	return dirwatch.New(root, isWorkspaceFile, onChange, logger,
		dirwatch.WithSubdirectories(),
		dirwatch.WithComponent("agentws-watcher"))
}

// isWorkspaceFile leaves out AGENTS.md, which nothing reads between opens.
func isWorkspaceFile(path string) bool {
	switch filepath.Base(path) {
	case libraryFileName, skillLibraryFileName, manifestFileName:
		return true
	}
	return false
}
