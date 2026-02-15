package doctor

import (
	"context"
	"os/exec"
)

// lookPathFunc is the function used to find executables on PATH.
// Package-level variable to allow test overrides.
var lookPathFunc = exec.LookPath

// ToolsCheck verifies that required external tools are available on $PATH.
type ToolsCheck struct{}

// NewToolsCheck creates a new tools check.
func NewToolsCheck() *ToolsCheck {
	return &ToolsCheck{}
}

func (c *ToolsCheck) Name() string {
	return "Tools"
}

func (c *ToolsCheck) Run(_ context.Context) Result {
	result := Result{Name: c.Name()}

	// git is required
	if path, err := lookPathFunc("git"); err != nil {
		result.Items = append(result.Items, CheckItem{
			Label:  "git",
			Status: StatusFail,
			Detail: "not found on PATH",
		})
	} else {
		result.Items = append(result.Items, CheckItem{
			Label:  "git",
			Status: StatusPass,
			Detail: path,
		})
	}

	// tmux is optional but recommended
	if path, err := lookPathFunc("tmux"); err != nil {
		result.Items = append(result.Items, CheckItem{
			Label:  "tmux",
			Status: StatusWarn,
			Detail: "not found on PATH (required for session spawn and preview)",
		})
	} else {
		result.Items = append(result.Items, CheckItem{
			Label:  "tmux",
			Status: StatusPass,
			Detail: path,
		})
	}

	return result
}
