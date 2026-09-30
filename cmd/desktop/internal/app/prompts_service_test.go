package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/prompts"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// isolateConfig points the desktop config paths at a temp dir so these tests
// never read or write the developer's real settings.
func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(settings.EnvConfigDir, dir)
	return dir
}

func newTestPromptsService(t *testing.T, port int) *PromptsService {
	t.Helper()
	b, err := settings.LoadBootstrap()
	require.NoError(t, err)
	paths := settings.ResolvePaths(b, settings.ResolveOptions{})
	store := settings.NewStore(paths.SettingsPath)
	return newPromptsService(paths, store, newWebhookService(WebhookDeps{Settings: store, Captures: nil, Listener: nil, Host: "127.0.0.1", Port: port}))
}

// TestCatalogRendersAgainstThisInstall is the reason prompts render in Go: a
// copied prompt has to name the paths on this machine.
func TestCatalogRendersAgainstThisInstall(t *testing.T) {
	dir := isolateConfig(t)
	svc := newTestPromptsService(t, 24917)

	catalog, err := svc.Catalog(t.Context(), prompts.Input{})
	require.NoError(t, err)
	require.NotEmpty(t, catalog)

	byID := make(map[string]prompts.Prompt, len(catalog))
	for _, prompt := range catalog {
		assert.NotEmptyf(t, prompt.Text, "prompt %q rendered empty", prompt.ID)
		assert.NotEmptyf(t, prompt.Title, "prompt %q has no title", prompt.ID)
		byID[prompt.ID] = prompt
	}

	assert.Contains(t, byID["flows"].Text, filepath.Join(dir, "flows"))
	assert.Contains(t, byID["actions"].Text, filepath.Join(dir, "actions.yml"))
	assert.Contains(t, byID["settings"].Text, filepath.Join(dir, "settings.yaml"))
	assert.Contains(t, byID["webhook-sources"].Text, "24917")
}

// TestCatalogSurvivesAnEmptyConfigRoot — the prompts page is most useful on a
// fresh install, before any config file exists.
func TestCatalogSurvivesAnEmptyConfigRoot(t *testing.T) {
	isolateConfig(t)
	catalog, err := newTestPromptsService(t, 0).Catalog(t.Context(), prompts.Input{})
	require.NoError(t, err)
	assert.NotEmpty(t, catalog)
}

func TestCatalogUsesConfiguredWebhookHost(t *testing.T) {
	isolateConfig(t)
	paths := settings.ResolvePaths(settings.Bootstrap{}, settings.ResolveOptions{})
	store := settings.NewStore(paths.SettingsPath)
	svc := newPromptsService(paths, store, newWebhookService(WebhookDeps{Settings: store, Captures: nil, Listener: nil, Host: "::1", Port: 24917}))

	catalog, err := svc.Catalog(t.Context(), prompts.Input{})
	require.NoError(t, err)
	for _, prompt := range catalog {
		if prompt.ID == "webhook-sources" {
			assert.Contains(t, prompt.Text, "http://[::1]:24917/hooks/")
			return
		}
	}
	t.Fatal("webhook-sources prompt not found")
}

func TestRenderReturnsContextScopedPrompts(t *testing.T) {
	isolateConfig(t)
	svc := newTestPromptsService(t, 24917)

	prompt, err := svc.Render(t.Context(), "webhook-transform", prompts.Input{
		WebhookPath:   "ci-alerts",
		WebhookSample: `{"event":"deploy"}`,
	})
	require.NoError(t, err)
	assert.Contains(t, prompt.Text, "ci-alerts")
	assert.Contains(t, prompt.Text, `{"event":"deploy"}`)

	_, err = svc.Render(t.Context(), "not-a-prompt", prompts.Input{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown prompt")
}
