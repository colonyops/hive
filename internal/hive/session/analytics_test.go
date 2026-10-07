package session

import (
	"context"
	"errors"
	"testing"

	"github.com/colonyops/hive/internal/config"
	sessiondomain "github.com/colonyops/hive/internal/domain/session"
	domain "github.com/colonyops/hive/internal/domain/usageanalytics"
	"github.com/stretchr/testify/require"
)

type analyticsCapture struct{ events []domain.Event }

func (c *analyticsCapture) Record(_ context.Context, event domain.Event) {
	c.events = append(c.events, event)
}

type pullFailureGit struct{ mockGit }

func (*pullFailureGit) Pull(context.Context, string) error { return errors.New("pull failed") }

func TestSessionAnalyticsSuccessFailureReuseAndFallback(t *testing.T) {
	for _, tt := range []struct {
		name        string
		recycled    bool
		pullFailure bool
		failSpawn   bool
		reused      bool
	}{
		{name: "new"}, {name: "reuse", recycled: true, reused: true}, {name: "pull fallback", recycled: true, pullFailure: true}, {name: "failed spawn", failSpawn: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{DataDir: t.TempDir(), GitPath: "git"}
			store := newMockStore()
			if tt.recycled {
				require.NoError(t, store.Save(t.Context(), sessiondomain.Session{ID: "old", Name: "old", Path: t.TempDir(), Remote: testRemote, State: sessiondomain.StateRecycled}))
			}
			svc := newTestService(t, store, cfg)
			capture := &analyticsCapture{}
			svc.analytics = capture
			if tt.pullFailure {
				svc.git = &pullFailureGit{}
			}
			_, err := svc.CreateSession(t.Context(), CreateOptions{Name: "private name", Remote: testRemote, SkipSpawn: !tt.failSpawn, AgentKey: "missing-profile"})
			if tt.failSpawn {
				require.Error(t, err)
				require.Empty(t, capture.events)
				return
			}
			require.NoError(t, err)
			require.Len(t, capture.events, 1)
			require.Equal(t, domain.SessionCreatedName, capture.events[0].Name())
			expected := `"reused_checkout":false`
			if tt.reused {
				expected = `"reused_checkout":true`
			}
			require.Contains(t, capture.events[0].Properties(), expected)
			require.NotContains(t, capture.events[0].Properties(), "private")
			require.NotContains(t, capture.events[0].Properties(), testRemote)
		})
	}
}
