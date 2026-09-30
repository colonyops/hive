// Package agentws owns the on-disk agent-workspace root: the directory tree
// holding mcps.yaml, .shared/, and one directory per workspace (spec §4).
package agentws

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// ErrRootUnavailable reports a configured root whose parent does not exist.
	ErrRootUnavailable = errors.New("agentws: workspace root unavailable")
	// ErrRootNotADirectory reports a root path occupied by a regular file.
	ErrRootNotADirectory = errors.New("agentws: workspace root is not a directory")
)

// EnsureRoot creates root when its parent exists, reporting whether this call
// created it. It reports ErrRootUnavailable when the parent does not exist and
// ErrRootNotADirectory when the path is occupied by a file.
//
// Auto-create applies only when the parent is reachable (spec §14): a
// configured root on an unmounted volume or a signed-out iCloud Drive is
// reported rather than silently replaced with a second empty root elsewhere.
func EnsureRoot(root string) (created bool, err error) {
	if info, statErr := os.Stat(root); statErr == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("%w: %s", ErrRootNotADirectory, root)
		}
		return false, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return false, fmt.Errorf("stat agent workspace root %s: %w", root, statErr)
	}

	parent := filepath.Dir(root)
	if _, statErr := os.Stat(parent); errors.Is(statErr, os.ErrNotExist) {
		return false, fmt.Errorf("%w: %s", ErrRootUnavailable, root)
	} else if statErr != nil {
		return false, fmt.Errorf("stat agent workspace root parent %s: %w", parent, statErr)
	}

	if err := os.Mkdir(root, 0o700); err != nil {
		return false, fmt.Errorf("create agent workspace root %s: %w", root, err)
	}
	return true, nil
}
