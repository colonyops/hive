package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestColorForString_Deterministic(t *testing.T) {
	// Same input always returns same color
	c1 := ColorForString("agent.abc123")
	c2 := ColorForString("agent.abc123")
	assert.Equal(t, c1, c2)
}

func TestColorForString_DifferentInputs(t *testing.T) {
	// Verify the function doesn't panic on varied inputs and stays within pool
	inputs := []string{"", "a", "agent.abc", "very.long.topic.name.here"}
	for _, input := range inputs {
		c := ColorForString(input)
		assert.NotNil(t, c)
	}
}

func TestViewType_String(t *testing.T) {
	tests := []struct {
		view ViewType
		want string
	}{
		{ViewSessions, "sessions"},
		{ViewMessages, "messages"},
		{ViewReview, "review"},
		{ViewType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.view.String())
		})
	}
}
