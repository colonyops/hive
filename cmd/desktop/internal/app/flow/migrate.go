package flow

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"

	configmigrate "github.com/colonyops/hive/internal/config/migrate"
)

// MigrateDir migrates every flow definition file under dir forward to current,
// writing a backup under backupDir for each rewrite. Skips .ui.yaml/.sidebar.yaml
// siblings via isFlowDefinition (the same filter LoadFlows uses). Per-file
// errors are logged, not fatal — matching flow's per-file-isolation semantics.
func MigrateDir(dir, backupDir string, log *zerolog.Logger) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read flows dir %q: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isFlowDefinition(name) {
			continue
		}
		path := filepath.Join(dir, name)
		if _, _, err := configmigrate.MigrateFile(configmigrate.FlowSet, path, backupDir, log); err != nil {
			log.Warn().Err(err).Str("file", path).Msg("flow migration failed")
		}
	}
	return nil
}
