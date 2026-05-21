package classifier

import (
	"context"
	"regexp"
	"time"

	"github.com/colonyops/hive/internal/core/terminal/process"
)

const (
	tierNone    = 0
	tierTitle   = 1
	tierProcess = 2
	tierContent = 3
	shellTool   = "shell"
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
	PaneID      string
	PanePID     int64
	WindowIndex string
	WindowName  string
	WorkDir     string
	Activity    int64
}

// Classifier classifies tmux panes as agent or non-agent.
type Classifier struct {
	titlePatterns []TitlePattern
	reader        process.ProcessReader
	capture       ContentCapture
	scorer        ContentScorer
}

// New creates a Classifier with the given dependencies.
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
	classifiedAt := time.Now()
	if tool, ok := c.classifyTitle(input.WindowName); ok {
		return Result{IsAgent: true, Tool: tool, Confidence: ConfidenceHigh, Tier: tierTitle, ClassifiedAt: classifiedAt}
	}
	if tool, ok := c.classifyProcess(input.PanePID); ok {
		return Result{IsAgent: true, Tool: tool, Confidence: ConfidenceHigh, Tier: tierProcess, ClassifiedAt: classifiedAt}
	}
	if tool, ok := c.classifyContent(ctx, input.PaneID); ok {
		return Result{IsAgent: true, Tool: tool, Confidence: ConfidenceMedium, Tier: tierContent, ClassifiedAt: classifiedAt}
	}
	return Result{Tier: tierNone, ClassifiedAt: classifiedAt}
}

func (c *Classifier) classifyTitle(windowName string) (tool string, ok bool) {
	for _, title := range c.titlePatterns {
		if title.Pattern == nil || !title.Pattern.MatchString(windowName) {
			continue
		}
		if title.Tool == "" {
			return "agent", true
		}
		return title.Tool, true
	}
	return "", false
}

func (c *Classifier) classifyProcess(panePID int64) (tool string, ok bool) {
	if panePID <= 0 {
		return "", false
	}
	proc, err := process.IdentifyWith(int(panePID), c.reader)
	if err != nil || proc == nil || proc.Tool == "" || proc.Tool == shellTool {
		return "", false
	}
	return proc.Tool, true
}

func (c *Classifier) classifyContent(ctx context.Context, paneID string) (tool string, ok bool) {
	if c.scorer == nil || c.capture == nil || paneID == "" {
		return "", false
	}
	content, err := c.capture.CapturePane(ctx, paneID)
	if err != nil || content == "" {
		return "", false
	}
	score, categories, tool := c.scorer.Score(content)
	if score < 6 || categories < 3 {
		return "", false
	}
	if tool == "" || tool == shellTool {
		tool = "agent"
	}
	return tool, true
}
