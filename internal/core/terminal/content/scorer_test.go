package content_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScorer_AgentContent(t *testing.T) {
	tests := []struct {
		fixture string
		tool    string
	}{
		{fixture: "claude_busy.txt", tool: "claude"},
		{fixture: "claude_ready.txt", tool: "claude"},
		{fixture: "claude_approval.txt", tool: "claude"},
		{fixture: "aider_session.txt", tool: "aider"},
	}
	scorer := content.NewScorer()
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			result := scorer.ScoreDetails(readFixture(t, tt.fixture))
			assert.True(t, result.IsAgent(), "score=%d categories=%d signals=%v", result.Score, result.Categories, result.Signals)
			assert.Equal(t, tt.tool, result.Tool)
		})
	}
}

func TestScorer_ShellContent(t *testing.T) {
	assertNotAgent(t, "shell_session.txt")
	assertNotAgent(t, "fancy_shell_prompt.txt")
}

func TestScorer_REPLContent(t *testing.T) {
	for _, fixture := range []string{"python_repl.txt", "node_repl.txt", "gdb_session.txt"} {
		t.Run(fixture, func(t *testing.T) { assertNotAgent(t, fixture) })
	}
}

func TestScorer_BuildLogContent(t *testing.T) { assertNotAgent(t, "build_log.txt") }
func TestScorer_PagerContent(t *testing.T)    { assertNotAgent(t, "pager_session.txt") }
func TestScorer_PackagePromptContent(t *testing.T) {
	assertNotAgent(t, "package_prompt.txt")
}

func TestScorer_ThresholdBoundary(t *testing.T) {
	scorer := content.NewScorer()
	below := "● assistant message\nRead(file.go)\n"
	result := scorer.ScoreDetails(below)
	assert.Equal(t, 4, result.Score)
	assert.False(t, result.IsAgent())

	atThreshold := below + "## Header\n❯\n"
	result = scorer.ScoreDetails(atThreshold)
	assert.Equal(t, 8, result.Score)
	assert.True(t, result.IsAgent())
}

func TestScorer_DiversityRequirement(t *testing.T) {
	scorer := content.NewScorer()
	result := scorer.ScoreDetails("ctrl+c to interrupt\n✳ thinking... (10s · 100 tokens)\n")
	assert.GreaterOrEqual(t, result.Score, 6)
	assert.Equal(t, 2, result.Categories)
	assert.False(t, result.IsAgent())
}

func TestScorer_EmptyContent(t *testing.T) {
	result := content.NewScorer().ScoreDetails(" \n\t ")
	assert.False(t, result.IsAgent())
	assert.Equal(t, 0, result.Score)
	assert.Equal(t, 0, result.Categories)
	assert.Equal(t, "shell", result.Tool)
}

func TestScoreSatisfiesClassifierInterface(t *testing.T) {
	score, categories, tool := content.NewScorer().Score(readFixture(t, "claude_busy.txt"))
	assert.GreaterOrEqual(t, score, 6)
	assert.GreaterOrEqual(t, categories, 3)
	assert.Equal(t, "claude", tool)
}

func assertNotAgent(t *testing.T, fixture string) {
	t.Helper()
	result := content.NewScorer().ScoreDetails(readFixture(t, fixture))
	assert.False(t, result.IsAgent(), "score=%d categories=%d signals=%v", result.Score, result.Categories, result.Signals)
}

func readFixture(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(data)
}
