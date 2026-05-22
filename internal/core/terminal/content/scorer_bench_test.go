package content_test

import (
	"strings"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/content"
)

func BenchmarkScorer_AgentContent(b *testing.B) {
	scorer := content.NewScorer()
	data := scorerFixtureContent("claude busy session")
	b.ReportAllocs()
	for b.Loop() {
		_ = scorer.ScoreDetails(data)
	}
}

func BenchmarkScorer_ShellContent(b *testing.B) {
	scorer := content.NewScorer()
	data := scorerFixtureContent("normal shell")
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
