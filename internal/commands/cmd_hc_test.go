package commands

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTimeAgo(t *testing.T) {
	tests := []struct {
		name  string
		input time.Time
		want  string
	}{
		{
			name:  "zero time returns never",
			input: time.Time{},
			want:  "never",
		},
		{
			name:  "recent past returns just now",
			input: time.Now().Add(-10 * time.Second),
			want:  "just now",
		},
		{
			name:  "minutes ago",
			input: time.Now().Add(-5 * time.Minute),
			want:  "5m ago",
		},
		{
			name:  "hours ago",
			input: time.Now().Add(-3 * time.Hour),
			want:  "3h ago",
		},
		{
			name:  "days ago",
			input: time.Now().Add(-48 * time.Hour),
			want:  "2d ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeAgo(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("non-zero time returns non-empty string", func(t *testing.T) {
		got := timeAgo(time.Now().Add(-1 * time.Minute))
		assert.NotEmpty(t, got)
	})
}
