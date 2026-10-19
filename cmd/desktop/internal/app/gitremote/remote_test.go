package gitremote

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	for _, tt := range []struct{ name, repo, url, https, ssh string }{
		{"github", "acme/site", "https://github.com/acme/site/issues/1", "https://github.com/acme/site.git", "git@github.com:acme/site.git"},
		{"enterprise", "acme/site.git", "http://git.example.test/acme/site/pull/1", "https://git.example.test/acme/site.git", "git@git.example.test:acme/site.git"},
		{"port", "acme/site", "https://git.example.test:8443/acme/site/issues/1", "https://git.example.test:8443/acme/site.git", "ssh://git@git.example.test:8443/acme/site.git"},
		{"ipv6", "acme/site", "https://[::1]:8443/acme/site", "https://[::1]:8443/acme/site.git", "ssh://git@[::1]:8443/acme/site.git"},
		{"ipv6 without port", "acme/site", "https://[::1]/acme/site", "https://[::1]/acme/site.git", "ssh://git@[::1]/acme/site.git"},
		{"missing host", "acme/site", "", "", ""},
		{"invalid url", "acme/site", "https://%broken", "", ""},
		{"non-web url", "acme/site", "file://local/a", "", ""},
		{"missing repo", "", "https://github.com", "", ""},
		{"incomplete repo", "acme/", "https://github.com", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.https, Resolve(tt.repo, tt.url, "https"))
			require.Equal(t, tt.ssh, Resolve(tt.repo, tt.url, "ssh"))
		})
	}
}

func TestResolvePreservesExplicitTargets(t *testing.T) {
	for _, target := range []string{
		"https://user@git.example.test:9443/group/repo.git",
		"ssh://custom@git.example.test:2222/group/repo.git",
		"git@github-work:group/repo.git", "host:group/repo.git",
		"git://git.example.test/group/repo", "file:///tmp/repo",
		"/tmp/repo", "./repo", "../repo", "~/code/repo", `C:\code\repo`,
	} {
		for _, protocol := range []string{"https", "ssh"} {
			require.Equal(t, target, Resolve(target, "https://github.com/group/repo/issues/1", protocol))
		}
	}
}
