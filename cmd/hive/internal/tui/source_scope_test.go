package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
