package agentws

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"

	configmigrate "github.com/colonyops/hive/internal/config/migrate"
)

// MigrateRoot migrates mcps.yaml and every workspace's agent-workspace.yaml
// under root forward to current, writing a backup under backupDir for each
// rewrite. Per-manifest errors are logged, not fatal, mirroring
// flow.MigrateDir: one unparseable workspace must not abort the sweep over
// its neighbours. A missing root is a no-op — it means EnsureRoot has not run
// yet, or nothing has ever been created.
func MigrateRoot(root, backupDir string, log *zerolog.Logger) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read agent workspace root %q: %w", root, err)
	}

	libPath := filepath.Join(root, libraryFileName)
	if _, _, err := configmigrate.MigrateFile(configmigrate.MCPLibrarySet, libPath, backupDir, log); err != nil {
		log.Warn().Err(err).Str("file", libPath).Msg("mcps.yaml migration failed")
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(root, entry.Name(), manifestFileName)
		if _, _, err := configmigrate.MigrateFile(configmigrate.AgentWorkspaceSet, manifestPath, backupDir, log); err != nil {
			log.Warn().Err(err).Str("file", manifestPath).Msg("agent-workspace.yaml migration failed")
		}
	}
	return nil
}
