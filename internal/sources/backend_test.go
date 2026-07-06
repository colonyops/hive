package sources_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/sources"
)

func TestDetectBackend(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		overrides map[string]sources.Backend
		want      sources.Backend
	}{
		{"github.com", "github.com", nil, sources.BackendGithub},
		{"codeberg is gitea", "codeberg.org", nil, sources.BackendGitea},
		{"gitea substring", "gitea.example.com", nil, sources.BackendGitea},
		{"forgejo substring", "code.forgejo.dev", nil, sources.BackendGitea},
		{"github enterprise substring", "github.acme.com", nil, sources.BackendGithub},
		{"unknown host defaults to github", "git.acme.com", nil, sources.BackendGithub},
		{"empty host defaults to github", "", nil, sources.BackendGithub},
		{"override wins over default", "git.acme.com", map[string]sources.Backend{"git.acme.com": sources.BackendGitea}, sources.BackendGitea},
		{"override wins over known host", "github.com", map[string]sources.Backend{"github.com": sources.BackendGitea}, sources.BackendGitea},
		{"override lookup is case-insensitive host", "GIT.ACME.COM", map[string]sources.Backend{"git.acme.com": sources.BackendGitea}, sources.BackendGitea},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sources.DetectBackend(tt.host, tt.overrides))
		})
	}
}
