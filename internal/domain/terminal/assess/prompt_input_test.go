package assess_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/domain/terminal/assess"
)

func TestPromptInput(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		want   string
		wantOK bool
	}{
		{"claude bare prompt with typed text", "claude/bare-prompt-typed.txt", "please refactor the auth module in internal/example/auth", true},
		{"claude bare prompt empty", "claude/bare-prompt-idle.txt", "", true},
		{"claude boxed prompt empty", "claude/idle-empty-box.txt", "", true},
		{"codex boxed prompt empty", "codex/idle-empty-box.txt", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := assess.PromptInput(fixtureContent(t, tt.file))
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPromptInputWithoutBox(t *testing.T) {
	_, ok := assess.PromptInput("just some shell output\n$ ")
	assert.False(t, ok)
}

func TestPromptInputWrapped(t *testing.T) {
	screen := "⏺ done\n\n" +
		"────────────────────\n" +
		"❯ Also add a div(a, b) function that raises ValueError on division by zero.\n" +
		"  Commit with message 'add div'. Then run: hive msg pub --topic\n" +
		"  orchestrator.test2 'div done'.\n" +
		"────────────────────\n" +
		"  feat/add-sub · 45k/1m 5% · model\n"
	got, ok := assess.PromptInput(screen)
	assert.True(t, ok)
	assert.Equal(t, "Also add a div(a, b) function that raises ValueError on division by zero. Commit with message 'add div'. Then run: hive msg pub --topic orchestrator.test2 'div done'.", got)
}
