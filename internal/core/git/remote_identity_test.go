package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoteIdentity_GitHubTransportsAreEquivalent(t *testing.T) {
	https := "https://github.com/owner/repo.git"
	ssh := "git@github.com:owner/repo.git"
	sshURL := "ssh://git@github.com/owner/repo.git"
	mixedCaseSCP := "git@GitHub.COM:Owner/Repo.git"
	mixedCaseSSHURL := "ssh://git@GITHUB.com/Owner/Repo.git"

	assert.Equal(t, "github.com/owner/repo", RemoteIdentity(https))
	assert.Equal(t, "github.com/owner/repo", RemoteIdentity(mixedCaseSCP))
	assert.True(t, EquivalentRemote(https, ssh))
	assert.True(t, EquivalentRemote(ssh, sshURL))
	assert.True(t, EquivalentRemote(mixedCaseSCP, mixedCaseSSHURL))
}

func TestRemoteIdentity_DoesNotConflateOtherRemotes(t *testing.T) {
	assert.False(t, EquivalentRemote("https://git.example.test/owner/repo.git", "ssh://git@git.example.test/owner/repo.git"))
	assert.False(t, EquivalentRemote("/repos/owner/repo", "/other/owner/repo"))
	assert.True(t, EquivalentRemote("/repos/owner/repo", "/repos/owner/repo"))
}
