//go:build remote

package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	assert.Contains(t, r.Stderr, "Attach with: ssh -t -F")
	assert.Contains(t, r.Stderr, "tmux attach -t 'windowed'")
	serverA.Shell(t, "tmux has-session -t windowed").Ok(t)
}
