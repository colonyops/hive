package usageanalytics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCatalogAndPrivacy(t *testing.T) {
	command, ok := LookupCommand("session.create")
	require.True(t, ok)
	for _, path := range []string{"session.create --remote secret", "/tmp/private", "help", "init", "completion", "user-command"} {
		_, ok := LookupCommand(path)
		require.False(t, ok, path)
	}
	events := []Event{CommandCompleted(command, false, 48*time.Hour), SessionCreated("full", true), TerminalStarted(false), TerminalStarted(true)}
	for _, e := range events {
		require.NoError(t, e.Validate())
		var properties map[string]any
		require.NoError(t, json.Unmarshal([]byte(e.Properties()), &properties))
		require.LessOrEqual(t, len(properties), 3)
		for _, forbidden := range []string{"path", "remote", "name", "prompt", "branch", "args", "content"} {
			require.NotContains(t, properties, forbidden)
		}
	}
	require.Contains(t, events[0].Properties(), `"duration_ms":86400000`)
	require.Contains(t, events[0].Properties(), `"outcome":"failure"`)
	require.Error(t, (Event{}).Validate())
	require.Error(t, SessionCreated("https://private.example", false).Validate())
	require.Error(t, CommandCompleted(Command{}, true, 0).Validate())
}
