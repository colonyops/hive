// Package beads provides a Beads issue tracker plugin for Hive.
package beads

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/plugins"
	"github.com/hay-kot/hive/internal/plugins/pluglib"
	"github.com/hay-kot/hive/internal/styles"
)

// Plugin implements the Beads plugin for Hive.
type Plugin struct {
	cfg       config.BeadsPluginConfig
	hasPerles bool
}

// New creates a new Beads plugin.
func New(cfg config.BeadsPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "beads" }

func (p *Plugin) Available() bool {
	// Check if user explicitly disabled
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	// Auto-detect: check if bd CLI is available
	_, err := exec.LookPath("bd")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error {
	// Detect if perles is available for better TUI
	_, err := exec.LookPath("perles")
	p.hasPerles = err == nil
	return nil
}

func (p *Plugin) Close() error { return nil }

func (p *Plugin) Commands() map[string]config.UserCommand {
	cmds := map[string]config.UserCommand{
		"BeadsReady": pluglib.TmuxPopup(`cd "{{ .Path }}" && bd ready {{ join .Args " " }}`, "show ready tasks [flags]"),
		"BeadsList":  pluglib.TmuxPopup(`cd "{{ .Path }}" && bd list --tree {{ join .Args " " }}`, "list all issues [flags]"),
	}

	// Add perles TUI command if available
	if p.hasPerles {
		cmds["BeadsTUI"] = config.UserCommand{
			Sh:     `tmux popup -E -w 95% -h 95% -- sh -c 'cd "{{ .Path }}" && perles'`,
			Help:   "open perles kanban TUI",
			Silent: true,
		}
	}

	return cmds
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return p
}

func (p *Plugin) RefreshStatus(ctx context.Context, sessions []*session.Session, pool *plugins.WorkerPool) (map[string]plugins.Status, error) {
	results := make(map[string]plugins.Status)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sess := range sessions {
		wg.Add(1)
		go func(s *session.Session) {
			defer wg.Done()
			pool.Run(func() {
				status := p.fetchBeadsStatus(ctx, s.Path)
				if status.Label != "" {
					mu.Lock()
					results[s.ID] = status
					mu.Unlock()
				}
			})
		}(sess)
	}

	wg.Wait()
	return results, nil
}

func (p *Plugin) fetchBeadsStatus(ctx context.Context, path string) plugins.Status {
	// Check if .beads directory exists
	beadsDir := filepath.Join(path, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return plugins.Status{}
	}

	// Count open issues
	openCmd := exec.CommandContext(ctx, "bd", "list", "--status=open")
	openCmd.Dir = path
	openOutput, _ := openCmd.Output()
	openCount := countLines(openOutput)

	// Count closed issues
	closedCmd := exec.CommandContext(ctx, "bd", "list", "--status=closed")
	closedCmd.Dir = path
	closedOutput, _ := closedCmd.Output()
	closedCount := countLines(closedOutput)

	total := openCount + closedCount
	if total == 0 {
		return plugins.Status{}
	}

	// Show closed/total (progress format)
	label := fmt.Sprintf("%d/%d", closedCount, total)

	var style lipgloss.Style
	switch {
	case closedCount == total:
		style = lipgloss.NewStyle().Foreground(styles.ColorGreen)
	case openCount > 0:
		style = lipgloss.NewStyle().Foreground(styles.ColorBlue)
	default:
		style = lipgloss.NewStyle().Foreground(styles.ColorGray)
	}

	return plugins.Status{
		Label: label,
		Icon:  "BD",
		Style: style,
	}
}

// countLines counts non-empty lines in output.
func countLines(output []byte) int {
	if len(output) == 0 {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func (p *Plugin) StatusCacheDuration() time.Duration {
	if p.cfg.ResultsCache > 0 {
		return p.cfg.ResultsCache
	}
	return 30 * time.Second
}
