package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/hive/internal/tui/command"
	"github.com/colonyops/hive/cmd/hive/internal/tui/views/sessions"
	"github.com/colonyops/hive/internal/platform/workspace"
)

func TestWithDiscoveredRepoForRemote_MatchesAnotherTransport(t *testing.T) {
	view := newMouseTestSessionsView(t)
	view.Update(sessions.RepositoriesDiscoveredMsg{Repositories: []workspace.DiscoveredRepo{
		{Name: "hive", Path: "/src/hive", Remote: "git@github.com:colonyops/hive.git"},
	}})
	m := Model{sessionsView: view}

	scope := m.withDiscoveredRepoForRemote(sourcePickerScope{Remote: "https://github.com/colonyops/hive"})
	assert.Equal(t, "/src/hive", scope.Source)
}

func TestStartCreate_CopiesFromTheChosenCheckout(t *testing.T) {
	creator := &fakeSessionCreator{}
	m := Model{cmdService: command.NewService(command.Deps{Creator: creator})}
	repo := workspace.DiscoveredRepo{Name: "beta", Path: "/src/beta", Remote: "git@github.com:user/beta.git"}

	started, ok := m.startCreate("fix-it", repo, "")().(streamStartedMsg)
	require.True(t, ok)
	for range started.output {
	}
	require.NoError(t, <-started.done)

	require.Len(t, creator.created, 1)
	assert.Equal(t, repo.Remote, creator.created[0].Remote)
	assert.Equal(t, repo.Path, creator.created[0].Source)
}
