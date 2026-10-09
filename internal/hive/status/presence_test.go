package status

import (
	"context"
	"testing"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type presenceIntegration struct {
	fakeTerminalIntegration
	present, known bool
	err            error
}

func (f *presenceIntegration) SessionPresence(context.Context, string, map[string]string) (bool, bool, error) {
	return f.present, f.known, f.err
}

func TestFetchSessionSeparatesAgentStatusFromSessionPresence(t *testing.T) {
	for _, tc := range []struct {
		name           string
		present, known bool
		err            error
	}{
		{name: "shell remains", present: true, known: true},
		{name: "session terminated", known: true},
		{name: "unknown transport state"},
		{name: "probe failed", err: assert.AnError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := terminal.NewManager([]string{"fake"})
			mgr.Register(&presenceIntegration{present: tc.present, known: tc.known, err: tc.err})
			status := NewService(zerolog.Nop(), mgr, 1).FetchSession(t.Context(), &session.Session{Slug: "agent"})
			assert.Equal(t, terminal.StatusMissing, status.Status)
			assert.Equal(t, tc.present, status.SessionPresent)
			assert.Equal(t, tc.known, status.PresenceKnown)
			if tc.err != nil {
				require.ErrorIs(t, status.Error, tc.err)
			} else {
				require.NoError(t, status.Error)
			}
		})
	}
}
