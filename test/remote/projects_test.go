//go:build remote

package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	projectdomain "github.com/colonyops/hive/internal/domain/project"
	projectsvc "github.com/colonyops/hive/internal/hive/project"
)

func TestProjectAPI_CreatesListsAndDeletesAProjectOnTheServer(t *testing.T) {
	api := serverA.API(t)
	ctx := t.Context()

	var steps []projectsvc.Step
	created, err := api.CreateProject(ctx, projectsvc.CreateOptions{
		Name:      "remote-project",
		Repos:     []projectsvc.RepoSpec{{Remote: tFixtureRepo, Dir: "app"}},
		SkipSpawn: true,
		OnStep:    func(step projectsvc.Step) { steps = append(steps, step) },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = api.DeleteProject(t.Context(), created.ID) })
	require.Len(t, created.Repos, 1)
	assert.Equal(t, "app", created.Repos[0].Dir)
	require.NotEmpty(t, steps, "the stream carries each step back")
	assert.Equal(t, projectsvc.StepDone, steps[len(steps)-1].State)

	listed := DecodeJSONLines[projectdomain.Session](t, serverA.Hive(t, "project", "list", "--json").Ok(t))
	require.Len(t, listed, 1)
	assert.Equal(t, created.ID, listed[0].ID)
	serverA.Shell(t, "test -L "+shellQuote(created.Root+"/app")).Ok(t)

	slug, found, err := api.ProjectOf(ctx, created.Repos[0].SessionID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, created.Slug, slug)

	require.NoError(t, api.DeleteProject(ctx, created.ID))
	assert.Empty(t, DecodeJSONLines[projectdomain.Session](t, serverA.Hive(t, "project", "list", "--json").Ok(t)))
	for _, sess := range serverA.List(t) {
		assert.NotEqual(t, created.Repos[0].SessionID, sess.ID, "the delete removes the repository's session too")
	}
}
