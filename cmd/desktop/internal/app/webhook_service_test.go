package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

func isolateSettings(t *testing.T) {
	t.Helper()
	t.Setenv(settings.EnvConfigDir, filepath.Join(t.TempDir(), "config"))
	t.Setenv(settings.EnvHTTPPort, "")
}

func testSettingsStore(t *testing.T) *settings.Store {
	t.Helper()
	return settings.NewStore(filepath.Join(t.TempDir(), "settings.yaml"))
}

func TestWebhookServiceInfoWithoutListener(t *testing.T) {
	service := newWebhookService(WebhookDeps{Settings: testSettingsStore(t), Captures: nil, Listener: nil, Host: "127.0.0.1", Port: 24483})
	running, port := service.Endpoint(t.Context())
	assert.False(t, running)
	assert.Equal(t, 24483, port)
	assert.Equal(t, "http://127.0.0.1:24483/hooks/", WebhookBaseURLAt(service.Host(), port))
}

func TestWebhookServiceCapture(t *testing.T) {
	db, err := queries.Open(t.Context(), t.TempDir(), queries.DefaultOpenOptions())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	captures := stores.New(db, stores.Options{}).WebhookCaptures
	service := newWebhookService(WebhookDeps{Settings: testSettingsStore(t), Captures: captures, Listener: nil, Host: "127.0.0.1", Port: 24483})

	view, err := service.Capture(t.Context(), "triage", "hook")
	require.NoError(t, err)
	assert.Zero(t, view.ReceivedAt)

	ctx := t.Context()
	require.NoError(t, captures.Upsert(ctx, "source:triage/hook", 42, []byte(`{"event":"deploy"}`)))
	view, err = service.Capture(t.Context(), "triage", "hook")
	require.NoError(t, err)
	assert.Equal(t, int64(42), view.ReceivedAt)
	assert.False(t, view.FeedShaped)
	assert.Equal(t, []string{"id", "kind", "repo", "title", "url"}, view.MissingFields)
}

func TestWebhookServiceSettingsDefaultEnabled(t *testing.T) {
	isolateSettings(t)
	service := newWebhookService(WebhookDeps{Settings: testSettingsStore(t), Captures: nil, Listener: nil, Host: "127.0.0.1", Port: 0})

	view, err := service.State(t.Context())
	require.NoError(t, err)
	assert.True(t, view.Enabled, "the loopback HTTP server is on by default")
	assert.Equal(t, "127.0.0.1", view.Host)
	assert.Zero(t, view.Port)
	assert.False(t, view.PortOverridden)
	assert.False(t, view.Running, "no listener was constructed for this test")
	assert.True(t, view.RestartRequired, "enabled in config but not yet running")
}

func TestWebhookServiceSettingsPortOverride(t *testing.T) {
	isolateSettings(t)
	t.Setenv(settings.EnvHTTPPort, "24499")
	service := newWebhookService(WebhookDeps{Settings: testSettingsStore(t), Captures: nil, Listener: nil, Host: "127.0.0.1", Port: 24499})

	view, err := service.State(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 24499, view.Port)
	assert.True(t, view.PortOverridden)
}

func TestWebhookServiceSetSettings(t *testing.T) {
	isolateSettings(t)
	service := newWebhookService(WebhookDeps{Settings: testSettingsStore(t), Captures: nil, Listener: nil, Host: "127.0.0.1", Port: 0})
	cfg := settings.DefaultSettings()
	cfg.Polling.Interval = settings.Duration(2 * time.Minute)
	require.NoError(t, settings.SaveSettings(cfg))

	require.NoError(t, service.SetState(t.Context(), true, "127.0.0.1", 27777))

	view, err := service.State(t.Context())
	require.NoError(t, err)
	assert.True(t, view.Enabled)
	assert.Equal(t, 27777, view.Port)
	assert.True(t, view.RestartRequired)

	saved, err := settings.LoadPersistedSettings()
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, saved.Polling.Interval.Duration())
}

func TestWebhookServiceSetSettingsValidation(t *testing.T) {
	isolateSettings(t)
	service := newWebhookService(WebhookDeps{Settings: testSettingsStore(t), Captures: nil, Listener: nil, Host: "127.0.0.1", Port: 0})

	for _, tc := range []struct {
		host string
		port int
	}{{"0.0.0.0", 0}, {"127.0.0.1", 80}, {"127.0.0.1", 65536}} {
		err := service.SetState(t.Context(), true, tc.host, tc.port)
		require.Error(t, err)
		require.Equal(t, KindInvalid, KindOf(err))
	}
	require.NoError(t, service.SetState(t.Context(), true, "127.0.0.1", 0))
}

func TestWebhookServiceGeneratePort(t *testing.T) {
	isolateSettings(t)
	service := newWebhookService(WebhookDeps{Settings: testSettingsStore(t), Captures: nil, Listener: nil, Host: "127.0.0.1", Port: 0})
	port, err := service.GeneratePort(t.Context())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, port, settings.WebhookPortMin)
	assert.LessOrEqual(t, port, settings.WebhookPortMax)
}
