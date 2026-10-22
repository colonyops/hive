package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/session"
)

func TestItemRepoLocation(t *testing.T) {
	tests := []struct {
		name, repo, itemURL, https, ssh string
	}{
		{"github", "acme/site", "https://github.com/acme/site/issues/1", "https://github.com/acme/site.git", "git@github.com:acme/site.git"},
		{"enterprise with .git", "acme/site.git", "https://git.example.test/acme/site/pull/1", "https://git.example.test/acme/site.git", "git@git.example.test:acme/site.git"},
		{"http item url", "acme/site", "http://git.example.test/acme/site/pull/1", "https://git.example.test/acme/site.git", "git@git.example.test:acme/site.git"},
		{"web port", "acme/site", "https://git.example.test:8443/acme/site/issues/1", "https://git.example.test:8443/acme/site.git", "git@git.example.test:acme/site.git"},
		{"default port", "acme/site", "https://github.com:443/acme/site", "https://github.com/acme/site.git", "git@github.com:acme/site.git"},
		{"subgroup", "group/sub/site", "https://gitlab.example.test/group/sub/site/-/issues/1", "https://gitlab.example.test/group/sub/site.git", "git@gitlab.example.test:group/sub/site.git"},
		{"ipv6", "acme/site", "https://[::1]:8443/acme/site", "https://[::1]:8443/acme/site.git", "git@[::1]:acme/site.git"},
		{"ipv6 without port", "acme/site", "https://[::1]/acme/site", "https://[::1]/acme/site.git", "git@[::1]:acme/site.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, ok := ItemRepoLocation(tt.repo, tt.itemURL)
			require.True(t, ok)
			assert.Equal(t, tt.https, loc.CloneURL(session.CloneProtocolHttps))
			assert.Equal(t, tt.ssh, loc.CloneURL(session.CloneProtocolSsh))
			assert.Equal(t, []string{tt.https, tt.ssh}, loc.Candidates())
		})
	}
}

func TestItemRepoLocationRejects(t *testing.T) {
	tests := []struct{ name, repo, itemURL string }{
		{"no host", "acme/site", ""},
		{"invalid url", "acme/site", "https://%broken"},
		{"file url", "acme/site", "file://local/a"},
		{"empty repo", "", "https://github.com"},
		{"one segment", "site", "https://github.com"},
		{"empty segment", "acme/", "https://github.com"},
		{"query in repo", "acme/site?x", "https://github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := ItemRepoLocation(tt.repo, tt.itemURL)
			assert.False(t, ok)
		})
	}
}

func TestIsExplicitRemote(t *testing.T) {
	for _, repo := range []string{
		"https://user@git.example.test:9443/group/repo.git",
		"ssh://custom@git.example.test:2222/group/repo.git",
		"git@github-work:group/repo.git", "host:group/repo.git",
		"git://git.example.test/group/repo", "file:///tmp/repo",
		"/tmp/repo", "./repo", "../repo", "~/code/repo", `C:\code\repo`,
	} {
		assert.True(t, IsExplicitRemote(repo), repo)
		_, ok := ItemRepoLocation(repo, "https://github.com/group/repo/issues/1")
		assert.False(t, ok, repo)
	}
	for _, repo := range []string{"acme/site", "group/sub/site"} {
		assert.False(t, IsExplicitRemote(repo), repo)
	}
}
