package content_test

import (
	"testing"

	"github.com/colonyops/hive/internal/core/terminal/content"
	"github.com/stretchr/testify/assert"
)

func TestScorer_FixtureCases(t *testing.T) {
	scorer := content.NewScorer()
	for _, tt := range scorerFixtures {
		t.Run(tt.name, func(t *testing.T) {
			result := scorer.ScoreDetails(tt.content)
			assert.Equal(t, tt.expectedAgent, result.IsAgent(), "score=%d categories=%d signals=%v purpose=%s", result.Score, result.Categories, result.Signals, tt.purpose)
			if tt.expectedTool != "" {
				assert.Equal(t, tt.expectedTool, result.Tool)
			}
		})
	}
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
	score, categories, tool := content.NewScorer().Score(scorerFixtureContent("claude busy session"))
	assert.GreaterOrEqual(t, score, 6)
	assert.GreaterOrEqual(t, categories, 3)
	assert.Equal(t, "claude", tool)
}
