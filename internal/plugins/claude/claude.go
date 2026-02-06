// Package claude provides Claude Code integration for Hive.
// It combines fork functionality and analytics in a single plugin.
package claude

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hay-kot/hive/internal/core/config"
	"github.com/hay-kot/hive/internal/core/session"
	"github.com/hay-kot/hive/internal/plugins"
	"github.com/hay-kot/hive/internal/styles"
)

// Plugin implements Claude Code integration (fork + analytics).
type Plugin struct {
	cfg   config.ClaudePluginConfig
	cache *Cache
}

// New creates a new claude plugin.
func New(cfg config.ClaudePluginConfig) *Plugin {
	cacheTTL := 30 * time.Second
	if cfg.CacheTTL > 0 {
		cacheTTL = cfg.CacheTTL
	}

	return &Plugin{
		cfg:   cfg,
		cache: NewCache(cacheTTL),
	}
}

func (p *Plugin) Name() string {
	return "claude"
}

func (p *Plugin) Available() bool {
	// Check if user explicitly disabled
	if p.cfg.Enabled != nil && !*p.cfg.Enabled {
		return false
	}
	// Check if claude CLI available
	_, err := exec.LookPath("claude")
	return err == nil
}

func (p *Plugin) Init(_ context.Context) error {
	return nil
}

func (p *Plugin) Close() error {
	return nil
}

func (p *Plugin) Commands() map[string]config.UserCommand {
	return map[string]config.UserCommand{
		"ClaudeFork": {
			Sh: `
# Fork current Claude session in new tmux window and focus it
cd "{{ .Path }}" && \
window_name="{{ .Name }}-fork" && \
tmux new-window -n "$window_name" -c "{{ .Path }}" \
  "exec claude --fork-session" && \
tmux select-window -t "$window_name"
`,
			Help:   "fork Claude session in new window",
			Silent: true,
		},
	}
}

func (p *Plugin) StatusProvider() plugins.StatusProvider {
	return p
}

// RefreshStatus implements plugins.StatusProvider
func (p *Plugin) RefreshStatus(ctx context.Context, sessions []*session.Session, pool *plugins.WorkerPool) (map[string]plugins.Status, error) {
	results := make(map[string]plugins.Status)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, sess := range sessions {
		// Only process Claude sessions
		claudeSessionID := sess.GetMeta("claude_session_id")
		if claudeSessionID == "" {
			continue
		}

		wg.Add(1)
		go func(s *session.Session, sessionID string) {
			defer wg.Done()

			pool.Run(func() {
				status := p.fetchAnalytics(ctx, s, sessionID)
				if status.Label != "" || status.Icon != "" {
					mu.Lock()
					results[s.ID] = status
					mu.Unlock()
				}
			})
		}(sess, claudeSessionID)
	}

	wg.Wait()
	return results, nil
}

func (p *Plugin) fetchAnalytics(ctx context.Context, s *session.Session, claudeSessionID string) plugins.Status {
	// Check cache first
	if cached := p.cache.Get(s.ID); cached != nil {
		return p.renderStatus(cached)
	}

	// Get JSONL path
	jsonlPath := GetClaudeJSONLPath(s.Path, claudeSessionID)
	if jsonlPath == "" {
		return plugins.Status{}
	}

	// Parse JSONL
	analytics, err := ParseSessionJSONL(jsonlPath)
	if err != nil {
		return plugins.Status{}
	}

	// Cache result
	p.cache.Set(s.ID, analytics)

	return p.renderStatus(analytics)
}

func (p *Plugin) renderStatus(a *SessionAnalytics) plugins.Status {
	// Get model limit (default 200k for Sonnet)
	modelLimit := 200000
	if p.cfg.ModelLimit > 0 {
		modelLimit = p.cfg.ModelLimit
	}

	percent := a.ContextPercent(modelLimit)

	// Two-tier threshold system: yellow → red
	yellowThreshold := 60 // Default: yellow at 60%
	if p.cfg.YellowThreshold > 0 {
		yellowThreshold = p.cfg.YellowThreshold
	}

	redThreshold := 80 // Default: red at 80%
	if p.cfg.RedThreshold > 0 {
		redThreshold = p.cfg.RedThreshold
	}

	var style lipgloss.Style
	switch {
	case percent >= float64(redThreshold):
		style = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	case percent >= float64(yellowThreshold):
		style = lipgloss.NewStyle().Foreground(styles.ColorYellow)
	default:
		// No color change (default text color)
		return plugins.Status{}
	}

	return plugins.Status{
		Label: "", // No label - just color change
		Icon:  "",
		Style: style,
	}
}

func (p *Plugin) StatusCacheDuration() time.Duration {
	if p.cfg.CacheTTL > 0 {
		return p.cfg.CacheTTL
	}
	return 30 * time.Second
}
