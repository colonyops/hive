package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WorkspacesCheck verifies that configured workspaces entries exist and are accessible.
type WorkspacesCheck struct {
	dirs []string
}

// NewWorkspacesCheck creates a new workspaces check.
func NewWorkspacesCheck(dirs []string) *WorkspacesCheck {
	return &WorkspacesCheck{dirs: dirs}
}

func (c *WorkspacesCheck) Name() string {
	return "Workspaces"
}

func (c *WorkspacesCheck) Run(_ context.Context) Result {
	result := Result{Name: c.Name()}

	if len(c.dirs) == 0 {
		result.Items = append(result.Items, CheckItem{
			Label:  "workspaces",
			Status: StatusPass,
			Detail: "none configured",
		})
		return result
	}

	for _, dir := range c.dirs {
		// Expand ~ to match runtime behavior (see ScanWorkspaces)
		if len(dir) > 0 && dir[0] == '~' {
			if home, err := os.UserHomeDir(); err == nil {
				dir = filepath.Join(home, dir[1:])
			}
		}

		info, err := os.Stat(dir)
		switch {
		case os.IsNotExist(err):
			result.Items = append(result.Items, CheckItem{
				Label:  dir,
				Status: StatusWarn,
				Detail: "directory does not exist",
			})
		case err != nil:
			result.Items = append(result.Items, CheckItem{
				Label:  dir,
				Status: StatusFail,
				Detail: fmt.Sprintf("inaccessible: %v", err),
			})
		case !info.IsDir():
			result.Items = append(result.Items, CheckItem{
				Label:  dir,
				Status: StatusFail,
				Detail: "path is not a directory",
			})
		default:
			result.Items = append(result.Items, CheckItem{
				Label:  dir,
				Status: StatusPass,
			})
		}
	}

	return result
}
