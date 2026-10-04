package commands

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
)

type fakeSessionLookup []session.Session

func (f fakeSessionLookup) GetSession(_ context.Context, id string) (session.Session, error) {
	for _, s := range f {
		if s.ID == id {
			return s, nil
		}
	}
	return session.Session{}, session.ErrNotFound
}

func (f fakeSessionLookup) ListSessions(context.Context) ([]session.Session, error) {
	return f, nil
}

func TestResolveSession(t *testing.T) {
	sessions := fakeSessionLookup{
		{ID: "a1", Name: "auth", Slug: "auth-x1", State: session.StateActive},
		{ID: "b2", Name: "api", Slug: "api-y2", State: session.StateActive},
		{ID: "c3", Name: "api", Slug: "api-z3", State: session.StateActive},
		{ID: "d4", Name: "docs", Slug: "docs-w4", State: session.StateRecycled},
	}
	tests := []struct {
		name    string
		key     string
		wantID  string
		wantErr string
	}{
		{"by ID", "c3", "c3", ""},
		{"by name", "auth", "a1", ""},
		{"by slug", "api-y2", "b2", ""},
		{"ambiguous name", "api", "", "2 active sessions are named"},
		{"inactive session is skipped", "docs", "", "no active session"},
		{"no match", "nope", "", "no active session"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSession(t.Context(), sessions, tt.key)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}
