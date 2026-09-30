package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/internal/app/skills"
)

func newTestSkillsService(t *testing.T) *SkillsService {
	t.Helper()
	b, err := settings.LoadBootstrap()
	require.NoError(t, err)
	paths := settings.ResolvePaths(b, settings.ResolveOptions{})
	store := settings.NewStore(paths.SettingsPath)
	promptsSvc := newPromptsService(paths, store, newWebhookService(WebhookDeps{Settings: store, Captures: nil, Listener: nil, Host: "127.0.0.1", Port: 24917}))
	return newSkillsService(promptsSvc)
}

func TestShippedSkillsNamesEveryListedPrompt(t *testing.T) {
	isolateConfig(t)
	svc := newTestSkillsService(t)

	shipped, err := svc.ShippedSkills(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, shipped)

	slugs := make(map[string]bool, len(shipped))
	for _, s := range shipped {
		assert.NotEmptyf(t, s.Title, "skill %q has no title", s.Slug)
		slugs[s.Slug] = true
	}
	assert.True(t, slugs["hive-flows"])
	// Touchpoint 12: a new prompt yields a shipped skill with no edit here.
	assert.True(t, slugs["hive-agent-workspaces"])
}

// Every shipped skill must render, because the workspace generator writes what
// ShippedSkills advertises — a slug the catalogue offers but RenderSkill
// refuses would fail the open of any workspace whose package selects it.
func TestEveryShippedSkillRenders(t *testing.T) {
	isolateConfig(t)
	svc := newTestSkillsService(t)

	shipped, err := svc.ShippedSkills(t.Context())
	require.NoError(t, err)
	for _, s := range shipped {
		id := strings.TrimPrefix(s.Slug, "hive-")
		name, body, err := svc.RenderSkill(t.Context(), id)
		require.NoErrorf(t, err, "skill %q", s.Slug)
		assert.Equal(t, s.Slug, name)
		assert.Truef(t, strings.HasPrefix(body, "---\nname: "+s.Slug+"\n"), "skill %q frontmatter:\n%s", s.Slug, body)
		require.NoError(t, skills.ValidateSkill(skills.Skill{ID: id, Name: name, Description: s.Description, Body: body}))
	}
}

func TestRenderSkillRejectsAnUnknownID(t *testing.T) {
	isolateConfig(t)
	svc := newTestSkillsService(t)

	_, _, err := svc.RenderSkill(t.Context(), "nope")
	require.Error(t, err)
}
