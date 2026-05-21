package content_test

import (
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/content"
)

func BenchmarkScorer_AgentContent(b *testing.B) {
	scorer := content.NewScorer()
	data := readFixture(b, "claude_busy.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = scorer.ScoreDetails(data)
	}
}

func BenchmarkScorer_ShellContent(b *testing.B) {
	scorer := content.NewScorer()
	data := readFixture(b, "shell_session.txt")
	b.ReportAllocs()
	for b.Loop() {
		_ = scorer.ScoreDetails(data)
	}
}

func BenchmarkScorer_LargeContent(b *testing.B) {
	scorer := content.NewScorer()
	data := strings.Repeat("user@host:~/project$ echo hello\nhello\n", 100)
	b.ReportAllocs()
	for b.Loop() {
		_ = scorer.ScoreDetails(data)
	}
}
