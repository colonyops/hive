//go:build remote

package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
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

func TestClientCLI_CreatePrintsHowToAttach(t *testing.T) {
	client := NewClient(t)

	r := client.HiveOn(serverA, "session", "create", "--json", "--remote", tFixtureRepo, "windowed").Ok(t)
	created := DecodeJSON[Session](t, r)
	serverA.DeleteOnCleanup(t, created.ID)

	assert.Contains(t, r.Stderr, "Attach with: hive session attach "+serverA.Name+":"+created.ID)
	serverA.Shell(t, "tmux has-session -t windowed").Ok(t)
}

func TestClientCLI_AttachRestartsAnEndedSessionAndReportsSSHFailures(t *testing.T) {
	client := NewClient(t)
	created := DecodeJSON[Session](t, client.HiveOn(serverA, "session", "create", "--json", "--background", "--remote", tFixtureRepo, "reattach").Ok(t))
	serverA.DeleteOnCleanup(t, created.ID)
	serverA.Shell(t, "tmux kill-session -t reattach").Ok(t)

	// The test has no terminal, so tmux on the server refuses the attach. The
	// start before it still runs, and the error carries ssh's last line.
	failed := client.Hive("session", "attach", serverA.Name+":"+created.ID).Fails(t)

	serverA.Shell(t, "tmux has-session -t reattach").Ok(t)
	assert.Contains(t, failed.Stdout+failed.Stderr, "ssh "+serverA.Name)
	assert.Contains(t, failed.Stdout+failed.Stderr, "not a terminal")
}

func TestSessionAPI_StartTmuxRefusesARecycledSession(t *testing.T) {
	created := serverA.CreateSession(t, "recycled-start")
	serverA.Hive(t, "session", "recycle", "--force", created.ID).Ok(t)

	_, err := serverA.API(t).StartTmuxSession(t.Context(), created.ID)

	require.ErrorIs(t, err, session.ErrNotActive)
}
