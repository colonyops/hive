package actions

import (
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dirwatch"
)

// NewActionsWatcher invokes onChange when actions.yml changes on disk, so
// edits made outside the app apply live.
func NewActionsWatcher(path string, onChange func(), logger zerolog.Logger) (*dirwatch.Watcher, error) {
	name := filepath.Base(path)
	return dirwatch.New(dirwatch.Config{
		Dir:       filepath.Dir(path),
		Match:     func(p string) bool { return filepath.Base(p) == name },
		Debounce:  250 * time.Millisecond,
		OnChange:  onChange,
		Component: "actions-watcher",
	}, logger)
}
