//go:build remote

package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCLI_CreateUsesTheServerAgentProfiles(t *testing.T) {
	client := NewClient(t)

	// The client config has no agents; testbash is a profile on the server.
	r := client.HiveOn(serverA, "session", "create", "--json", "--background", "--remote", tFixtureRepo, "--agent", "testbash", "server-agent").Ok(t)
	serverA.DeleteOnCleanup(t, DecodeJSON[Session](t, r).ID)

	failed := client.HiveOn(serverA, "session", "create", "--background", "--remote", tFixtureRepo, "--agent", "nope", "no-agent").Fails(t)
	// hive prints its errors to stdout.
	assert.Contains(t, failed.Stdout+failed.Stderr, `server server-a has no agent "nope"`)
}

func TestClientCLI_CreatePrintsHowToAttachWhenTheServerMakesATmuxSession(t *testing.T) {
	client := NewClient(t)

	r := client.HiveOn(serverA, "session", "create", "--json", "--remote", tWindowsRepo, "windowed").Ok(t)
	created := DecodeJSON[Session](t, r)
	serverA.DeleteOnCleanup(t, created.ID)

	assert.Contains(t, r.Stderr, "Attach with: ssh -t -F")
	assert.Contains(t, r.Stderr, "tmux attach -t 'windowed'")
	serverA.Shell(t, "tmux has-session -t windowed").Ok(t)

	// The legacy spawn rule runs a command the server cannot attach to, so the
	// client prints nothing.
	r = client.HiveOn(serverA, "session", "create", "--json", "--remote", tFixtureRepo, "legacy").Ok(t)
	serverA.DeleteOnCleanup(t, DecodeJSON[Session](t, r).ID)
	assert.NotContains(t, r.Stderr, "Attach with")
}

func TestSessionAPI_SpawnPlanFollowsTheServerRules(t *testing.T) {
	api := serverA.API(t)

	windows, err := api.SpawnCreatesTmuxSession(t.Context(), tWindowsRepo)
	require.NoError(t, err)
	assert.True(t, windows)
	legacy, err := api.SpawnCreatesTmuxSession(t.Context(), tFixtureRepo)
	require.NoError(t, err)
	assert.False(t, legacy)
}
