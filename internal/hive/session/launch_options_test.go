package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type launchOptionsGit struct {
	mockGit
	remote string
}

func (g *launchOptionsGit) RemoteURL(context.Context, string) (string, error) { return g.remote, nil }

func TestSessionLaunchOptions_PrefersConfiguredCheckoutForEquivalentGitHubRemote(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspaces")
	checkout := filepath.Join(workspaceDir, "hive")
	require.NoError(t, mkdirGitDir(checkout))
	hiveClone := filepath.Join(t.TempDir(), "hive-clone")
	require.NoError(t, mkdirGitDir(hiveClone))

	store := newMockStore()
	store.sessions["existing"] = session.Session{ID: "existing", Name: "existing hive", Path: hiveClone, Remote: "ssh://git@github.com/colonyops/hive.git"}
	cfg := &config.Config{
		DataDir:    t.TempDir(),
		Workspaces: []string{workspaceDir},
		Agents: config.AgentsConfig{Default: "claude", Profiles: map[string]config.AgentProfile{
			"pi": {}, "claude": {},
		}},
	}
	service := newTestService(t, store, cfg)
	service.git = &launchOptionsGit{remote: "https://github.com/colonyops/hive.git"}

	options, err := service.SessionLaunchOptions(t.Context())
	require.NoError(t, err)
	require.Equal(t, []LaunchRepository{{Name: "hive", Remote: "https://github.com/colonyops/hive.git", Source: checkout}}, options.Repositories)
	assert.Equal(t, "https://github.com/colonyops/hive.git", options.DefaultRepository)
	assert.Equal(t, []string{"claude", "pi"}, options.Agents)

	resolved, err := service.ResolveSessionLaunchRepository(t.Context(), "git@github.com:colonyops/hive.git")
	require.NoError(t, err)
	assert.Equal(t, options.Repositories[0], resolved)
}

func TestSessionLaunchOptions_PreservesExplicitLocalRemote(t *testing.T) {
	store := newMockStore()
	service := newTestService(t, store, &config.Config{DataDir: t.TempDir()})
	resolved, err := service.ResolveSessionLaunchRepository(t.Context(), "/Users/me/src/hive")
	require.NoError(t, err)
	assert.Equal(t, LaunchRepository{Remote: "/Users/me/src/hive"}, resolved)
}

func TestResolveSessionLaunchRepository_PreferenceDecidesSpelling(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspaces")
	checkout := filepath.Join(workspaceDir, "hive")
	require.NoError(t, mkdirGitDir(checkout))

	cfg := &config.Config{
		DataDir:    t.TempDir(),
		Workspaces: []string{workspaceDir},
		Git:        config.GitConfig{CloneProtocol: session.CloneProtocolSsh},
	}
	service := newTestService(t, newMockStore(), cfg)
	service.git = &launchOptionsGit{remote: "https://github.com/colonyops/hive.git"}

	resolved, err := service.ResolveSessionLaunchRepository(t.Context(), "git@github.com:colonyops/hive.git")
	require.NoError(t, err)
	assert.Equal(t, LaunchRepository{Name: "hive", Remote: "git@github.com:colonyops/hive.git", Source: checkout}, resolved,
		"the request's spelling is cloned and the checkout supplies the source")
}

func TestResolveSessionLaunchRepository_RuleForTheCheckoutSpellingApplies(t *testing.T) {
	workspaceDir := filepath.Join(t.TempDir(), "workspaces")
	checkout := filepath.Join(workspaceDir, "site")
	require.NoError(t, mkdirGitDir(checkout))

	cfg := &config.Config{
		DataDir:    t.TempDir(),
		Workspaces: []string{workspaceDir},
		Rules:      []config.Rule{{Pattern: `^https://github\.com/acme/`, CloneProtocol: session.CloneProtocolSsh}},
	}
	service := newTestService(t, newMockStore(), cfg)
	service.git = &launchOptionsGit{remote: "https://github.com/acme/site.git"}

	resolved, err := service.ResolveSessionLaunchRepository(t.Context(), "git@github.com:acme/site.git")
	require.NoError(t, err)
	assert.Equal(t, "git@github.com:acme/site.git", resolved.Remote)
}

func TestPreferredRemote(t *testing.T) {
	const itemURL = "https://github.com/acme/site/issues/1"
	ssh := config.Config{Git: config.GitConfig{CloneProtocol: session.CloneProtocolSsh}}

	for name, tc := range map[string]struct {
		cfg  config.Config
		repo string
		want string
	}{
		"no preference is empty":   {config.Config{}, "acme/site", ""},
		"preference generates":     {ssh, "acme/site", "git@github.com:acme/site.git"},
		"explicit remote is empty": {ssh, "https://example.com/a/b.git", ""},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.DataDir = t.TempDir()
			service := newTestService(t, newMockStore(), &cfg)
			assert.Equal(t, tc.want, service.PreferredRemote(tc.repo, itemURL))
		})
	}
}

func TestGeneratedRemote(t *testing.T) {
	const itemURL = "https://github.com/acme/site/issues/1"
	const giteaURL = "https://gitea.example.com/acme/site/issues/1"

	tests := []struct {
		name    string
		cfg     config.Config
		repo    string
		itemURL string
		want    string
	}{
		{"unset is https", config.Config{}, "acme/site", itemURL, "https://github.com/acme/site.git"},
		{"global ssh", config.Config{Git: config.GitConfig{CloneProtocol: session.CloneProtocolSsh}}, "acme/site", itemURL, "git@github.com:acme/site.git"},
		{
			"host rule written for https returns to https",
			config.Config{
				Git:   config.GitConfig{CloneProtocol: session.CloneProtocolSsh},
				Rules: []config.Rule{{Pattern: `.*[:/]gitea\.example\.com[:/].*`, CloneProtocol: session.CloneProtocolHttps}},
			},
			"acme/site", giteaURL, "https://gitea.example.com/acme/site.git",
		},
		{
			"rule written for ssh matches",
			config.Config{Rules: []config.Rule{{Pattern: `^git@github\.com:acme/`, CloneProtocol: session.CloneProtocolSsh}}},
			"acme/site", itemURL, "git@github.com:acme/site.git",
		},
		{"explicit passes through trimmed", config.Config{Git: config.GitConfig{CloneProtocol: session.CloneProtocolSsh}}, " https://example.com/a/b.git ", itemURL, "https://example.com/a/b.git"},
		{"unexpandable is empty", config.Config{}, "acme/site", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			cfg.DataDir = t.TempDir()
			service := newTestService(t, newMockStore(), &cfg)
			assert.Equal(t, tt.want, service.GeneratedRemote(tt.repo, tt.itemURL))
		})
	}
}

func mkdirGitDir(path string) error {
	return os.MkdirAll(filepath.Join(path, ".git"), 0o755)
}
