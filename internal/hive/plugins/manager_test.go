package plugins

import (
	"testing"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/stretchr/testify/assert"
)

func TestSessionsChanged(t *testing.T) {
	mkSession := func(id string) *session.Session {
		return &session.Session{ID: id}
	}

	tests := []struct {
		name string
		old  []*session.Session
		new  []*session.Session
		want bool
	}{
		{
			name: "both nil",
			old:  nil,
			new:  nil,
			want: false,
		},
		{
			name: "same single session",
			old:  []*session.Session{mkSession("a")},
			new:  []*session.Session{mkSession("a")},
			want: false,
		},
		{
			name: "same sessions different order",
			old:  []*session.Session{mkSession("a"), mkSession("b")},
			new:  []*session.Session{mkSession("b"), mkSession("a")},
			want: false,
		},
		{
			name: "session added",
			old:  []*session.Session{mkSession("a")},
			new:  []*session.Session{mkSession("a"), mkSession("b")},
			want: true,
		},
		{
			name: "session removed",
			old:  []*session.Session{mkSession("a"), mkSession("b")},
			new:  []*session.Session{mkSession("a")},
			want: true,
		},
		{
			name: "session replaced",
			old:  []*session.Session{mkSession("a")},
			new:  []*session.Session{mkSession("b")},
			want: true,
		},
		{
			name: "nil to non-empty",
			old:  nil,
			new:  []*session.Session{mkSession("a")},
			want: true,
		},
		{
			name: "non-empty to nil",
			old:  []*session.Session{mkSession("a")},
			new:  nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionsChanged(tt.old, tt.new)
			assert.Equal(t, tt.want, got)
		})
	}
}
