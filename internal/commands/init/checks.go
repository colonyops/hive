package initcmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hay-kot/hive/internal/commands/doctor"
)

// InitCheck validates the init wizard results.
type InitCheck struct {
	configPath string
}

// NewInitCheck creates a new init validation check.
func NewInitCheck(configPath string) *InitCheck {
	return &InitCheck{configPath: configPath}
}

func (c *InitCheck) Name() string {
	return "Init Validation"
}

func (c *InitCheck) Run(ctx context.Context) doctor.Result {
	result := doctor.Result{Name: c.Name()}

	// Check config file exists
	result.Items = append(result.Items, c.checkConfigFile())

	// Check hive.sh script
	result.Items = append(result.Items, c.checkHiveScript())

	// Check tmux availability
	result.Items = append(result.Items, c.checkTmux())

	// Check git availability
	result.Items = append(result.Items, c.checkGit())

	// Check PATH includes ~/.local/bin
	result.Items = append(result.Items, c.checkPath())

	return result
}

func (c *InitCheck) checkConfigFile() doctor.CheckItem {
	if _, err := os.Stat(c.configPath); err != nil {
		return doctor.CheckItem{
			Label:  "Config file",
			Status: doctor.StatusFail,
			Detail: c.configPath + " not found",
		}
	}
	return doctor.CheckItem{
		Label:  "Config file",
		Status: doctor.StatusPass,
		Detail: c.configPath,
	}
}

func (c *InitCheck) checkHiveScript() doctor.CheckItem {
	path := ScriptPath()
	info, err := os.Stat(path)
	if err != nil {
		return doctor.CheckItem{
			Label:  "hive.sh script",
			Status: doctor.StatusWarn,
			Detail: "not installed",
		}
	}
	if info.Mode()&0o100 == 0 {
		return doctor.CheckItem{
			Label:  "hive.sh script",
			Status: doctor.StatusWarn,
			Detail: "not executable",
		}
	}
	return doctor.CheckItem{
		Label:  "hive.sh script",
		Status: doctor.StatusPass,
		Detail: path,
	}
}

func (c *InitCheck) checkTmux() doctor.CheckItem {
	if TmuxAvailable() {
		return doctor.CheckItem{
			Label:  "tmux",
			Status: doctor.StatusPass,
			Detail: "available",
		}
	}
	return doctor.CheckItem{
		Label:  "tmux",
		Status: doctor.StatusWarn,
		Detail: "not found - terminal integration disabled",
	}
}

func (c *InitCheck) checkGit() doctor.CheckItem {
	if _, err := exec.LookPath("git"); err == nil {
		return doctor.CheckItem{
			Label:  "git",
			Status: doctor.StatusPass,
			Detail: "available",
		}
	}
	return doctor.CheckItem{
		Label:  "git",
		Status: doctor.StatusFail,
		Detail: "not found - hive requires git",
	}
}

func (c *InitCheck) checkPath() doctor.CheckItem {
	home, _ := os.UserHomeDir()
	localBin := filepath.Join(home, ".local", "bin")

	pathEnv := os.Getenv("PATH")
	paths := strings.Split(pathEnv, string(os.PathListSeparator))

	if slices.Contains(paths, localBin) {
		return doctor.CheckItem{
			Label:  "PATH",
			Status: doctor.StatusPass,
			Detail: "includes " + localBin,
		}
	}

	return doctor.CheckItem{
		Label:  "PATH",
		Status: doctor.StatusWarn,
		Detail: localBin + " not in PATH",
	}
}
