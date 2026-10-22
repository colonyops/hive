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
	userlessSCP := "GITHUB.COM:Owner/Repo.GIT"

	assert.Equal(t, "github.com/owner/repo", RemoteIdentity(https))
	assert.Equal(t, "github.com/owner/repo", RemoteIdentity(mixedCaseSCP))
	assert.Equal(t, "github.com/owner/repo", RemoteIdentity(userlessSCP))
	assert.True(t, EquivalentRemote(https, ssh))
	assert.True(t, EquivalentRemote(ssh, sshURL))
	assert.True(t, EquivalentRemote(mixedCaseSCP, mixedCaseSSHURL))
	assert.True(t, EquivalentRemote(https, userlessSCP))
}

func TestRemoteIdentity_OtherForgeTransportsAreEquivalent(t *testing.T) {
	for _, remote := range []string{
		"https://git.example.test/team/sub/Repo.git",
		"ssh://git@git.example.test/team/sub/Repo.git",
		"ssh://git@git.example.test:22/team/sub/Repo",
		"git://GIT.example.test/team/sub/Repo.git/",
		"git@git.example.test:team/sub/Repo.git",
		"git.example.test:team/sub/Repo",
	} {
		assert.Equal(t, "git.example.test/team/sub/Repo", RemoteIdentity(remote), remote)
	}
}

func TestRemoteIdentity_KeepsDistinctRepositoriesApart(t *testing.T) {
	assert.False(t, EquivalentRemote("git@github.com:owner/repo.git", "git@gitlab.com:owner/repo.git"))
	assert.False(t, EquivalentRemote("https://git.example.test/owner/Repo", "https://git.example.test/owner/repo"))
	assert.False(t, EquivalentRemote("ssh://git@git.example.test:2222/owner/repo", "git@git.example.test:owner/repo"))
	assert.False(t, EquivalentRemote("/repos/owner/repo", "/other/owner/repo"))
	assert.True(t, EquivalentRemote("/repos/owner/repo", "/repos/owner/repo"))
	assert.False(t, EquivalentRemote("", ""))
	assert.False(t, EquivalentRemote("alice@git.example.test:repo.git", "bob@git.example.test:repo.git"))
	assert.True(t, EquivalentRemote("alice@git.example.test:/srv/repo.git", "ssh://bob@git.example.test/srv/repo"))
}

func TestRemoteIdentity_ReturnsUnrecognizedRemotesUnchanged(t *testing.T) {
	for _, remote := range []string{"/repos/project.git", "./a:b", "owner/repo", "file:///repos/project.git", "https://", "https://github.com/"} {
		assert.Equal(t, remote, RemoteIdentity(remote), remote)
	}
	assert.Equal(t, "example.test:2222/owner/repo", RemoteIdentity("ssh://git@example.test:2222/owner/repo.git"))
}
