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
	"github.com/hay-kot/hive/internal/core/kv"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/core/styles"
	"github.com/hay-kot/hive/internal/hive/plugins"
	"github.com/hay-kot/hive/internal/hive/plugins/pluglib"
)

// Plugin implements the GitHub plugin for Hive.
type Plugin struct {
	cfg   config.GitHubPluginConfig
	cache *kv.TypedKV[prInfo]
}

// New creates a new GitHub plugin.
// If kvStore is non-nil, PR status is cached in the persistent KV store.
func New(cfg config.GitHubPluginConfig, kvStore kv.KV) *Plugin {
	p := &Plugin{cfg: cfg}
	if kvStore != nil {
		p.cache = kv.Scoped[prInfo](kvStore, "github.pr")
	}
	return p
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
				info := p.fetchPRInfo(ctx, s.ID, s.Path)
				status := infoToStatus(info)
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

// fetchPRInfo returns PR info, checking the cache first.
func (p *Plugin) fetchPRInfo(ctx context.Context, sessionID, path string) prInfo {
	// Try cache first
	if p.cache != nil {
		if cached, err := p.cache.Get(ctx, sessionID); err == nil {
			return cached
		}
	}

	// Cache miss - fetch from gh CLI
	info := p.fetchFromGH(ctx, path)

	// Store in cache with TTL
	if p.cache != nil && info.Number > 0 {
		_ = p.cache.SetTTL(ctx, sessionID, info, p.StatusCacheDuration())
	}

	return info
}

func (p *Plugin) fetchFromGH(ctx context.Context, path string) prInfo {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", "--json", "number,state,isDraft")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return prInfo{}
	}

	var info prInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return prInfo{}
	}

	return info
}

func infoToStatus(info prInfo) plugins.Status {
	if info.Number == 0 {
		return plugins.Status{}
	}

	var label string
	var style lipgloss.Style

	if info.IsDraft {
		label = "draft"
		style = lipgloss.NewStyle().Foreground(styles.ColorMuted)
	} else {
		switch info.State {
		case "OPEN":
			label = "open"
			style = lipgloss.NewStyle().Foreground(styles.ColorSuccess)
		case "MERGED":
			label = "merged"
			style = lipgloss.NewStyle().Foreground(styles.ColorPrimary)
		case "CLOSED":
			label = "closed"
			style = lipgloss.NewStyle().Foreground(styles.ColorMuted)
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
	return 2 * time.Minute
}
