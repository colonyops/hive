package doctor

import (
	"context"
	"fmt"
	"os"
)

// RepoDirsCheck verifies that configured repo_dirs entries exist and are accessible.
type RepoDirsCheck struct {
	dirs []string
}

// NewRepoDirsCheck creates a new repo directories check.
func NewRepoDirsCheck(dirs []string) *RepoDirsCheck {
	return &RepoDirsCheck{dirs: dirs}
}

func (c *RepoDirsCheck) Name() string {
	return "Repository Directories"
}

func (c *RepoDirsCheck) Run(_ context.Context) Result {
	result := Result{Name: c.Name()}

	if len(c.dirs) == 0 {
		result.Items = append(result.Items, CheckItem{
			Label:  "repo_dirs",
			Status: StatusPass,
			Detail: "none configured",
		})
		return result
	}

	for _, dir := range c.dirs {
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
