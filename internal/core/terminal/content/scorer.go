// Package content scores terminal content for agent-like structure.
package content

import (
	"strings"

	"github.com/colonyops/hive/internal/core/terminal"
)

const (
	agentScoreThreshold      = 6
	agentCategoriesThreshold = 3
	unknownAgentTool         = "agent"
	shellTool                = "shell"
)

// Signal represents a detected content signal.
type Signal struct {
	Category string
	Weight   int
	Pattern  string
}

// ScoreResult holds the full scoring output.
type ScoreResult struct {
	Score      int
	Categories int
	Signals    []Signal
	Tool       string
}

// IsAgent reports whether the result meets the agent classification threshold.
func (r ScoreResult) IsAgent() bool {
	return r.Score >= agentScoreThreshold && r.Categories >= agentCategoriesThreshold
}

// Scorer evaluates terminal content for agent-like structural signals.
type Scorer struct {
	positiveRules []rule
	negativeRules []rule
}

// NewScorer creates a scorer with default positive and negative rules.
func NewScorer() *Scorer {
	return &Scorer{
		positiveRules: defaultPositiveRules(),
		negativeRules: defaultNegativeRules(),
	}
}

// Score satisfies classifier.ContentScorer.
func (s *Scorer) Score(content string) (int, int, string) {
	r := s.ScoreDetails(content)
	return r.Score, r.Categories, r.Tool
}

// ScoreDetails returns the full scoring result for debugging and tests.
func (s *Scorer) ScoreDetails(content string) ScoreResult {
	if s == nil {
		s = NewScorer()
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return ScoreResult{Tool: shellTool}
	}

	lines := normalizedLines(content)
	result := ScoreResult{Tool: detectedTool(content)}
	positiveCategories := make(map[string]bool)

	for _, r := range s.positiveRules {
		if pattern, ok := r.Match(lines); ok {
			result.Score += r.Weight
			result.Signals = append(result.Signals, Signal{Category: r.Category, Weight: r.Weight, Pattern: pattern})
			if r.Weight > 0 {
				positiveCategories[r.Category] = true
			}
		}
	}
	for _, r := range s.negativeRules {
		if pattern, ok := r.Match(lines); ok {
			result.Score += r.Weight
			result.Signals = append(result.Signals, Signal{Category: r.Category, Weight: r.Weight, Pattern: pattern})
		}
	}
	result.Categories = len(positiveCategories)
	return result
}

func normalizedLines(content string) []string {
	raw := strings.Split(terminal.StripANSI(content), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.ReplaceAll(line, "\u00A0", " ")
		lines = append(lines, line)
	}
	return lines
}

func detectedTool(content string) string {
	tool := terminal.DetectTool(content)
	if tool == "" || tool == shellTool {
		return shellTool
	}
	return tool
}
