package terminal

import (
	"regexp"
	"testing"
)

const benchToolClaude = "claude"

func BenchmarkDetectTool(b *testing.B) {
	content := `Welcome to Claude Code

✳ thinking... (12s · 1.2k tokens)
Read(internal/core/terminal/detector.go)
ctrl+c to interrupt
`
	b.ReportAllocs()
	for b.Loop() {
		if got := DetectTool(content); got != benchToolClaude {
			b.Fatalf("DetectTool() = %q", got)
		}
	}
}

func BenchmarkTitleRegexMatch(b *testing.B) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)claude`),
		regexp.MustCompile(`(?i)codex`),
		regexp.MustCompile(`(?i)gemini`),
		regexp.MustCompile(`(?i)aider`),
	}
	windowTitle := "hive-agent-claude-main"
	b.ReportAllocs()
	for b.Loop() {
		matched := false
		for _, pattern := range patterns {
			if pattern.MatchString(windowTitle) {
				matched = true
				break
			}
		}
		if !matched {
			b.Fatal("title did not match")
		}
	}
}
