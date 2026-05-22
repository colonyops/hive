package classifier

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/terminal/content"
	"github.com/colonyops/hive/internal/core/terminal/process"
)

const (
	tierNone    = 0
	tierTitle   = 1
	tierProcess = 2
	tierContent = 3

	// envClaudeCode is injected by the Claude Code SDK (CLAUDECODE=1). It is
	// a reliable agent signal even when argv is obscured by macOS hardened runtime.
	envClaudeCode = "CLAUDECODE"
)

// ContentCapture abstracts pane content retrieval for Tier 3.
type ContentCapture interface {
	CapturePane(ctx context.Context, target string) (string, error)
}

// ContentScorer scores terminal content for agent-like signals.
type ContentScorer interface {
	Score(content string) (score int, categories int, tool string)
}

// TitlePattern is a compiled regex for Tier 1 title matching.
type TitlePattern struct {
	Pattern *regexp.Regexp
	Tool    string
}

// PaneInput holds the raw data for one pane from tmux list-panes.
type PaneInput struct {
	SessionName string
	PaneID      string
	PanePID     int64
	WindowIndex string
	WindowName  string
	PaneTitle   string
	WorkDir     string
	Activity    int64
	HiveSession string
}

// Classifier classifies tmux panes as agent or non-agent.
type Classifier struct {
	titlePatterns []TitlePattern
	reader        process.ProcessReader
	capture       ContentCapture
	scorer        ContentScorer
}

// WithReader returns a shallow copy of the Classifier that uses r for process
// identification instead of the original reader. Use this to inject a
// per-refresh-cycle SnapshotReader without mutating the shared Classifier.
func (c *Classifier) WithReader(r process.ProcessReader) *Classifier {
	copy := *c
	copy.reader = r
	return &copy
}

// New creates a Classifier with the given dependencies.
// titlePatterns drives both Tier 1 (pane title) and Tier 2 (process binary
// name) detection, so no separate tool-name list is required.
func New(titles []TitlePattern, reader process.ProcessReader, capture ContentCapture, scorer ContentScorer) *Classifier {
	if reader == nil {
		reader = process.OSReader{}
	}
	return &Classifier{
		titlePatterns: titles,
		reader:        reader,
		capture:       capture,
		scorer:        scorer,
	}
}

// Classify runs the 3-tier cascade on a single pane.
func (c *Classifier) Classify(ctx context.Context, input PaneInput) Result {
	return c.classify(ctx, input, true)
}

// ClassifyStable runs only Tier 1 (title) and Tier 2 (process) classification.
// It skips Tier 3 content capture, which spawns external subprocesses. Use
// this during periodic cache refresh when Tier 3 is gated by a rate limiter.
func (c *Classifier) ClassifyStable(input PaneInput) Result {
	return c.classify(context.Background(), input, false)
}

func (c *Classifier) classify(ctx context.Context, input PaneInput, allowContent bool) Result {
	classifiedAt := time.Now()
	if tool, ok := c.classifyTitle(input.PaneTitle); ok {
		return Result{IsAgent: true, Tool: tool, Confidence: ConfidenceHigh, Tier: tierTitle, ClassifiedAt: classifiedAt}
	}
	if tool, ok := c.classifyProcess(input.PanePID); ok {
		return Result{IsAgent: true, Tool: tool, Confidence: ConfidenceHigh, Tier: tierProcess, ClassifiedAt: classifiedAt}
	}
	if allowContent {
		if tool, ok := c.classifyContent(ctx, input.PaneID); ok {
			return Result{IsAgent: true, Tool: tool, Confidence: ConfidenceMedium, Tier: tierContent, ClassifiedAt: classifiedAt}
		}
	}
	return Result{Tier: tierNone, ClassifiedAt: classifiedAt}
}

func (c *Classifier) classifyTitle(paneTitle string) (tool string, ok bool) {
	for _, title := range c.titlePatterns {
		if title.Pattern == nil || !title.Pattern.MatchString(paneTitle) {
			continue
		}
		if title.Tool == "" {
			return "agent", true
		}
		return title.Tool, true
	}
	return "", false
}

// classifyProcess fetches the candidate processes for panePID and checks each
// one against the compiled title patterns. Argv parsing (npx, python -m, etc.)
// is handled here so the process package stays focused on OS introspection.
func (c *Classifier) classifyProcess(panePID int64) (tool string, ok bool) {
	if panePID <= 0 {
		return "", false
	}
	candidates, err := process.Candidates(int(panePID), c.reader)
	if err != nil || candidates == nil {
		return "", false
	}

	if tool := c.matchProcessInfo(candidates.Foreground); tool != "" {
		return tool, true
	}
	for _, child := range candidates.Children {
		if tool := c.matchProcessInfo(child); tool != "" {
			return tool, true
		}
	}
	return "", false
}

// matchProcessInfo checks a single process against the classifier's patterns.
func (c *Classifier) matchProcessInfo(info process.ProcessInfo) string {
	// CLAUDECODE=1 is set by the Claude Code SDK; treat it as a definitive
	// signal before pattern matching in case argv is obscured.
	if info.Env[envClaudeCode] == "1" {
		if tool := c.matchName("claude"); tool != "" {
			return tool
		}
		return "claude"
	}

	// Match the binary name directly.
	if tool := c.matchName(info.Comm); tool != "" {
		return tool
	}

	// Match through argv: direct binary path, then launcher wrappers.
	return c.matchArgv(info.Argv)
}

// matchName checks a single name (binary basename or module name) against
// every compiled title pattern and returns the tool name on the first match.
func (c *Classifier) matchName(name string) string {
	lower := strings.ToLower(name)
	for _, tp := range c.titlePatterns {
		if tp.Pattern != nil && tp.Pattern.MatchString(lower) {
			if tp.Tool == "" {
				return "agent"
			}
			return tp.Tool
		}
	}
	return ""
}

// matchArgv checks argv[0] basename and handles common launcher wrappers
// (npx, python -m, mise, env) so that invocations like "npx claude" or
// "python3 -m aider" are detected via the same title patterns.
//
// Interactive shells (bash -lc "…", zsh -c "…") are intentionally excluded:
// any agent they launch will appear as a child process and be picked up by
// the Candidates walk instead.
func (c *Classifier) matchArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	base0 := strings.ToLower(filepath.Base(argv[0]))

	if tool := c.matchName(base0); tool != "" {
		return tool
	}

	switch base0 {
	case "node", "npx", "npm", "pnpm", "yarn", "bun", "uvx":
		return c.matchExecutableArgs(argv[1:])
	case "python", "python2", "python3":
		return c.matchPythonArgs(argv[1:])
	case "mise":
		return c.matchMiseArgs(argv[1:])
	case "env":
		return c.matchEnvArgs(argv[1:])
	}
	return ""
}

func (c *Classifier) matchExecutableArgs(args []string) string {
	for _, arg := range args {
		if isArgvFlag(arg) {
			continue
		}
		if tool := c.matchName(filepath.Base(arg)); tool != "" {
			return tool
		}
	}
	return ""
}

func (c *Classifier) matchPythonArgs(args []string) string {
	for i, arg := range args {
		if arg == "-m" && i+1 < len(args) {
			return c.matchName(filepath.Base(args[i+1]))
		}
	}
	return ""
}

func (c *Classifier) matchMiseArgs(args []string) string {
	for i, arg := range args {
		if arg == "--" && i+1 < len(args) {
			return c.matchName(filepath.Base(args[i+1]))
		}
	}
	return c.matchExecutableArgs(args)
}

func (c *Classifier) matchEnvArgs(args []string) string {
	for _, arg := range args {
		if strings.Contains(arg, "=") || strings.HasPrefix(arg, "-") {
			continue
		}
		return c.matchName(filepath.Base(arg))
	}
	return ""
}

func isArgvFlag(arg string) bool {
	return arg == "" || arg == "--" || strings.HasPrefix(arg, "-")
}

func (c *Classifier) classifyContent(ctx context.Context, paneID string) (tool string, ok bool) {
	if c.scorer == nil || c.capture == nil || paneID == "" {
		return "", false
	}
	paneContent, err := c.capture.CapturePane(ctx, paneID)
	if err != nil || paneContent == "" {
		return "", false
	}
	score, categories, tool := c.scorer.Score(paneContent)
	if score < content.AgentScoreThreshold || categories < content.AgentCategoriesThreshold {
		return "", false
	}
	if tool == "" {
		tool = "agent"
	}
	return tool, true
}
