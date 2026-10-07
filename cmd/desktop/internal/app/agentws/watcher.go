package agentws

import (
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dirwatch"
)

// NewWatcher invokes onChange when the workspace root changes on disk:
// mcps.yaml or skills.yml at the root, agent-workspace.yaml inside a workspace
// directory, or a workspace directory appearing or disappearing. Nothing
// deeper is watched: an agent writing into docs/, or the generator rewriting
// CLAUDE.md/.mcp.json/.codex/ on open, is invisible to it by design.
func NewWatcher(root string, onChange func(), logger zerolog.Logger) (*dirwatch.Watcher, error) {
	return dirwatch.New(root, isWorkspaceFile, onChange, logger,
		dirwatch.WithSubdirectories(),
		dirwatch.WithComponent("agentws-watcher"))
}

// isWorkspaceFile leaves out AGENTS.md, which nothing reads between opens,
// and the generator's own outputs beside agent-workspace.yaml.
func isWorkspaceFile(path string) bool {
	switch filepath.Base(path) {
	case libraryFileName, skillLibraryFileName, manifestFileName:
		return true
	}
	return false
}
