// Package github provides a GitHub plugin for Hive.
package github

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/plugins"
	"github.com/hay-kot/hive/internal/plugins/pluglib"
	"github.com/hay-kot/hive/internal/styles"
)

// Plugin implements the GitHub plugin for Hive.
type Plugin struct {
	cfg config.GitHubPluginConfig
}

// New creates a new GitHub plugin.
func New(cfg config.GitHubPluginConfig) *Plugin {
	return &Plugin{cfg: cfg}
}

func (p *Plugin) Name() string { return "github" }

func (p *Plugin) Available() bool {
	// Check if user explicitly disabled
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	// Auto-detect: check if gh CLI is available
	_, err := exec.LookPath("gh")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error { return nil }
func (p *Plugin) Close() error                 { return nil }

func (p *Plugin) Commands() map[string]config.UserCommand {
	return map[string]config.UserCommand{
		"GithubOpenRepo": {Sh: "cd {{ .Path }} && gh browse", Help: "open repo in browser"},
		"GithubOpenPR":   {Sh: "cd {{ .Path }} && gh pr view --web", Help: "view current PR in browser"},
		"GithubPRStatus": pluglib.TmuxPopup(`cd "{{ .Path }}" && gh pr status {{ join .Args " " }}`, "show PR status [flags]"),
		"GithubPRCreate": {Sh: "cd {{ .Path }} && gh pr create --web", Help: "create PR in browser"},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return p
}

// prInfo represents GitHub PR information from gh CLI.
type prInfo struct {
	Number  int    `json:"number"`
	State   string `json:"state"`
	IsDraft bool   `json:"isDraft"`
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
				status := p.fetchPRStatus(ctx, s.Path)
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

func (p *Plugin) fetchPRStatus(ctx context.Context, path string) plugins.Status {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", "--json", "number,state,isDraft")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		// No PR for this branch - not an error, just empty status
		return plugins.Status{}
	}

	var info prInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return plugins.Status{}
	}

	// Build status display
	var label string
	var style lipgloss.Style

	if info.IsDraft {
		label = "draft"
		style = lipgloss.NewStyle().Foreground(styles.ColorGray)
	} else {
		switch info.State {
		case "OPEN":
			label = "open"
			style = lipgloss.NewStyle().Foreground(styles.ColorGreen)
		case "MERGED":
			label = "merged"
			style = lipgloss.NewStyle().Foreground(styles.ColorBlue)
		case "CLOSED":
			label = "closed"
			style = lipgloss.NewStyle().Foreground(styles.ColorGray)
		default:
			label = info.State
			style = lipgloss.NewStyle()
		}
	}

	return plugins.Status{
		Label: label,
		Icon:  "PR",
		Style: style,
	}
}

func (p *Plugin) StatusCacheDuration() time.Duration {
	if p.cfg.ResultsCache > 0 {
		return p.cfg.ResultsCache
	}
	return 30 * time.Second
}
