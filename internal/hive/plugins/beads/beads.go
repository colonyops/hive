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
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/kv"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive/plugins"
	"github.com/colonyops/hive/internal/hive/plugins/pluglib"
)

// beadsInfo holds cached issue counts for a session.
type beadsInfo struct {
	OpenCount   int `json:"openCount"`
	ClosedCount int `json:"closedCount"`
}

// Plugin implements the Beads plugin for Hive.
type Plugin struct {
	cfg       config.BeadsPluginConfig
	cache     *kv.TypedKV[beadsInfo]
	hasPerles bool
}

// New creates a new Beads plugin.
func New(cfg config.BeadsPluginConfig, kvStore kv.KV) *Plugin {
	p := &Plugin{cfg: cfg}
	if kvStore != nil {
		p.cache = kv.Scoped[beadsInfo](kvStore, "beads.status")
	}
	return p
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
				status := p.fetchBeadsStatus(ctx, s.ID, s.Path)
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

func (p *Plugin) fetchBeadsStatus(ctx context.Context, sessionID, path string) plugins.Status {
	// Try cache first
	if p.cache != nil {
		if cached, err := p.cache.Get(ctx, sessionID); err == nil {
			return p.renderStatus(cached)
		}
	}

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

	info := beadsInfo{OpenCount: openCount, ClosedCount: closedCount}

	// Cache result
	if p.cache != nil {
		_ = p.cache.SetTTL(ctx, sessionID, info, p.StatusCacheDuration())
	}

	return p.renderStatus(info)
}

func (p *Plugin) renderStatus(info beadsInfo) plugins.Status {
	total := info.OpenCount + info.ClosedCount
	if total == 0 {
		return plugins.Status{}
	}

	label := fmt.Sprintf("%d/%d", info.ClosedCount, total)

	var style lipgloss.Style
	switch {
	case info.ClosedCount == total:
		style = lipgloss.NewStyle().Foreground(styles.ColorSuccess)
	case info.OpenCount > 0:
		style = lipgloss.NewStyle().Foreground(styles.ColorPrimary)
	default:
		style = lipgloss.NewStyle().Foreground(styles.ColorMuted)
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
