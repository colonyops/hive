package hive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/session"
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
	require.Equal(t, []SessionLaunchRepository{{Name: "hive", Remote: "https://github.com/colonyops/hive.git", Source: checkout}}, options.Repositories)
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
	assert.Equal(t, SessionLaunchRepository{Remote: "/Users/me/src/hive"}, resolved)
}

func mkdirGitDir(path string) error {
	return os.MkdirAll(filepath.Join(path, ".git"), 0o755)
}
