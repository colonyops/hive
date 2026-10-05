package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
)

func isolateSettings(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)
	return filepath.Join(dir, settingsFileName)
}

func TestDefaultSettingsAreSafe(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, cfg.Polling.Interval.Duration())
	assert.True(t, cfg.Updates.Enabled)
	assert.True(t, cfg.Notifications.Enabled)
	assert.Equal(t, DeliveryAuto, cfg.Notifications.Delivery)
	assert.True(t, cfg.HTTP.Enabled, "the loopback HTTP server is on by default")
	assert.Equal(t, "127.0.0.1", cfg.HTTP.Host)
	assert.Zero(t, cfg.HTTP.Port)
	assert.Equal(t, MockLive, cfg.Development.Mocks.Mode)
	assert.False(t, cfg.Development.Pprof.Enabled)
	assert.True(t, cfg.Appearance.TerminalShowStatusBar, "the session status bar ships on")
	assert.False(t, cfg.Appearance.TerminalShowSessionAge, "session age badges are opt-in")
	assert.Equal(t, DefaultTerminalSessionAgeThresholdDays, cfg.Appearance.TerminalSessionAgeThresholdDays)
}

// The graduated features' gate is gone from the struct; the migration drops
// the section so the file does not warn about an unknown key
// (ADR terminal-agents-grafana-and-commands-graduate-out-of-experimental).
func TestRetiredExperimentalSectionIsMigratedAway(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("experimental:\n  terminal: true\n  agents: true\n"), 0o600))

	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, configmigrate.SettingsSet.Current, cfg.Version)
	assert.NoError(t, cfg.UnknownKeys())
}

// Every save wrote an explicit false while the bar shipped off, so the flip to
// on has to reach a saved file through the migration, not just the default.
func TestPersistedTerminalStatusBarOffIsMigratedOn(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("version: 3\nappearance:\n  terminal_show_windows: true\n  terminal_show_status_bar: false\n  terminal_pool_size: 3\n"), 0o600))

	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.True(t, cfg.Appearance.TerminalShowStatusBar)
	assert.Equal(t, configmigrate.SettingsSet.Current, cfg.Version)
}

// The flip is a one-time reset: a file already at the current version that
// says off stays off.
func TestTerminalStatusBarOffAtCurrentVersionIsKept(t *testing.T) {
	path := isolateSettings(t)
	raw := "version: " + strconv.Itoa(configmigrate.SettingsSet.Current) + "\nappearance:\n  terminal_show_status_bar: false\n"
	require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))

	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.False(t, cfg.Appearance.TerminalShowStatusBar)
}

func TestTerminalSessionAgeThresholdValidation(t *testing.T) {
	for _, days := range []int{MinTerminalSessionAgeThresholdDays - 1, MaxTerminalSessionAgeThresholdDays + 1} {
		cfg := DefaultSettings()
		cfg.Appearance.TerminalSessionAgeThresholdDays = days
		require.ErrorContains(t, cfg.Validate(), "appearance.terminal_session_age_threshold_days")
	}

	cfg := DefaultSettings()
	cfg.Appearance.TerminalSessionAgeThresholdDays = MaxTerminalSessionAgeThresholdDays
	require.NoError(t, cfg.Validate())
}

func TestProfilesOrderRoundTrips(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("profiles:\n  order: [personal, hive]\n"), 0o600))

	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, []string{"personal", "hive"}, cfg.Profiles.Order)

	require.NoError(t, SaveSettings(cfg))
	cfg, err = LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, []string{"personal", "hive"}, cfg.Profiles.Order,
		"a save of unrelated settings must not drop the rail order")
}

// An unset order writes no section at all, so a settings.yaml that never asked
// for a rail order does not grow an empty one on the next save.
func TestUnsetProfilesOrderIsNotSerialized(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, SaveSettings(DefaultSettings()))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "profiles:")
}

func TestPathsTmuxYAMLThenEnvironment(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("paths:\n  tmux: /opt/homebrew/bin/tmux\n"), 0o600))

	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, "/opt/homebrew/bin/tmux", cfg.Paths.Tmux)

	t.Setenv("HIVE_DESKTOP_PATHS_TMUX", "/usr/local/bin/tmux")
	cfg, err = LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin/tmux", cfg.Paths.Tmux)
	assert.True(t, cfg.EnvironmentOverridden("HIVE_DESKTOP_PATHS_TMUX"))
}

func TestLoadSettingsNestedYAMLThenEnvironment(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte(`
polling:
  interval: 2m
updates:
  enabled: false
notifications:
  enabled: true
  delivery: app
  sound: false
http:
  enabled: true
  host: 127.0.0.1
  port: 24001
development:
  mocks:
    mode: pipeline
  vite:
    host: 127.0.0.1
    port: 0
  wails:
    host: 127.0.0.1
    port: 0
  pprof:
    enabled: false
  debug:
    pause_ingest: 0s
    pause_commit: 0s
`), 0o600))
	t.Setenv("HIVE_DESKTOP_HTTP_PORT", "25002")
	t.Setenv("HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE", "feed")
	t.Setenv("HIVE_DESKTOP_DEVELOPMENT_VITE_PORT", "43123")

	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, cfg.Polling.Interval.Duration())
	assert.False(t, cfg.Updates.Enabled)
	assert.Equal(t, 25002, cfg.HTTP.Port)
	assert.Equal(t, MockFeed, cfg.Development.Mocks.Mode)
	assert.Equal(t, 43123, cfg.Development.Vite.Port)
	assert.True(t, cfg.EnvironmentOverridden(EnvHTTPPort))

	persisted, err := LoadPersistedSettings()
	require.NoError(t, err)
	assert.Equal(t, 24001, persisted.HTTP.Port)
	assert.Equal(t, MockPipeline, persisted.Development.Mocks.Mode)
}

func TestLoadSettingsReportsUnknownFields(t *testing.T) {
	path := isolateSettings(t)
	contents := "http:\n  enabled: false\n  typo: true\nmenu_bar:\n  feeds:\n    - feed: flow/node\n      future: 1\ntelemetry_v2:\n  endpoint: x\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.False(t, cfg.HTTP.Enabled)
	assert.Equal(t, "flow/node", cfg.MenuBar.Feeds[0].Feed)
	require.Error(t, cfg.UnknownKeys())
	for _, key := range []string{"field typo not found", "field future not found", "field telemetry_v2 not found"} {
		assert.ErrorContains(t, cfg.UnknownKeys(), key)
	}
}

func TestLoadSettingsReportsNoUnknownFieldsForAKnownFile(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("http:\n  enabled: false\n"), 0o600))
	cfg, err := LoadSettings()
	require.NoError(t, err)
	assert.NoError(t, cfg.UnknownKeys())
}

func TestLoadSettingsStillRejectsAWrongTypedValue(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("http:\n  port: not-a-port\n  typo: true\n"), 0o600))
	_, err := LoadSettings()
	require.ErrorContains(t, err, "parse desktop settings")
}

func TestLoadSettingsRejectsInvalidEnvironment(t *testing.T) {
	isolateSettings(t)
	t.Setenv("HIVE_DESKTOP_HTTP_PORT", "not-a-port")
	_, err := LoadSettings()
	require.ErrorContains(t, err, "parse error on field \"Port\"")
}

func TestLoadSettingsRejectsUnsupportedViteHostEnvironment(t *testing.T) {
	isolateSettings(t)
	t.Setenv("HIVE_DESKTOP_DEVELOPMENT_VITE_HOST", "127.0.0.2")

	_, err := LoadSettings()
	require.ErrorContains(t, err, "development.vite.host must be 127.0.0.1")
}

func TestLoadSettingsRejectsInvalidPersistedValueShadowedByEnvironment(t *testing.T) {
	path := isolateSettings(t)
	require.NoError(t, os.WriteFile(path, []byte("polling:\n  interval: 1s\n"), 0o600))
	t.Setenv("HIVE_DESKTOP_POLLING_INTERVAL", "5m")

	_, err := LoadSettings()
	require.ErrorContains(t, err, "validate persisted desktop settings: polling.interval")
}

func TestSaveSettingsDoesNotPersistEnvironmentOverride(t *testing.T) {
	path := isolateSettings(t)
	base := DefaultSettings()
	base.HTTP.Port = 24001
	require.NoError(t, SaveSettings(base))
	t.Setenv(EnvHTTPPort, "25002")

	effective, err := LoadSettings()
	require.NoError(t, err)
	assert.Equal(t, 25002, effective.HTTP.Port)

	persisted, err := LoadPersistedSettings()
	require.NoError(t, err)
	persisted.Appearance.Theme = "dark"
	require.NoError(t, SaveSettings(persisted))

	reloaded, err := LoadPersistedSettings()
	require.NoError(t, err)
	assert.Equal(t, 24001, reloaded.HTTP.Port)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "25002")
}

func TestSettingsValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Settings)
	}{
		{"short poll", func(s *Settings) { s.Polling.Interval = Duration(time.Second) }},
		{"update channel", func(s *Settings) { s.Updates.Channel = "nightly" }},
		{"delivery", func(s *Settings) { s.Notifications.Delivery = "desktop" }},
		{"http host", func(s *Settings) { s.HTTP.Host = "0.0.0.0" }},
		{"http port", func(s *Settings) { s.HTTP.Port = 80 }},
		{"mock", func(s *Settings) { s.Development.Mocks.Mode = "mystery" }},
		{"vite host", func(s *Settings) { s.Development.Vite.Host = "127.0.0.2" }},
		{"negative pause", func(s *Settings) { s.Development.Debug.PauseCommit = Duration(-time.Second) }},
		{"excessive pause", func(s *Settings) { s.Development.Debug.PauseIngest = Duration(MaxDebugPause + time.Second) }},
		{"github api base remote host", func(s *Settings) {
			s.Development.GitHub.APIBase = "https://api.github.example.com"
		}},
		{"github api base public ip", func(s *Settings) {
			s.Development.GitHub.APIBase = "http://10.0.0.5:8080"
		}},
		{"github api base scheme", func(s *Settings) {
			s.Development.GitHub.APIBase = "ftp://127.0.0.1:8080"
		}},
		{"github api base missing scheme", func(s *Settings) {
			s.Development.GitHub.APIBase = "127.0.0.1:8080"
		}},
		{"relative tmux path", func(s *Settings) { s.Paths.Tmux = "bin/tmux" }},
		{"bare tmux name", func(s *Settings) { s.Paths.Tmux = "tmux" }},
		{"telemetry without endpoint", func(s *Settings) {
			s.Telemetry = telemetryFixture(func(t *TelemetrySettings) { t.Endpoint = "" })
		}},
		{"telemetry without instance id", func(s *Settings) {
			s.Telemetry = telemetryFixture(func(t *TelemetrySettings) { t.InstanceID = "" })
		}},
		{"telemetry plaintext endpoint", func(s *Settings) {
			s.Telemetry = telemetryFixture(func(t *TelemetrySettings) { t.Endpoint = "http://gw.example.com/otlp" })
		}},
		{"telemetry endpoint without host", func(s *Settings) {
			s.Telemetry = telemetryFixture(func(t *TelemetrySettings) { t.Endpoint = "https:///otlp" })
		}},
		{"telemetry without token", func(s *Settings) {
			s.Telemetry = telemetryFixture(func(t *TelemetrySettings) { t.Token = "" })
		}},
		// A pasted credential is the thing config must never hold, so it is
		// rejected rather than passed through as a literal.
		{"telemetry literal token", func(s *Settings) {
			s.Telemetry = telemetryFixture(func(t *TelemetrySettings) { t.Token = "glc_eyJvIjoiMTIzNDU2In0=" })
		}},
		{"telemetry unknown token prefix", func(s *Settings) {
			s.Telemetry = telemetryFixture(func(t *TelemetrySettings) { t.Token = "vault:kv/data/otlp" })
		}},
		{"profiles without endpoint", func(s *Settings) {
			s.Telemetry.Profiles = profileTelemetryFixture(func(t *ProfileTelemetrySettings) { t.Endpoint = "" })
		}},
		{"profiles without user", func(s *Settings) {
			s.Telemetry.Profiles = profileTelemetryFixture(func(t *ProfileTelemetrySettings) { t.User = "" })
		}},
		{"profiles plaintext endpoint", func(s *Settings) {
			s.Telemetry.Profiles = profileTelemetryFixture(func(t *ProfileTelemetrySettings) { t.Endpoint = "http://profiles.example.com" })
		}},
		{"profiles endpoint without host", func(s *Settings) {
			s.Telemetry.Profiles = profileTelemetryFixture(func(t *ProfileTelemetrySettings) { t.Endpoint = "https:///ingest" })
		}},
		{"profiles without token", func(s *Settings) {
			s.Telemetry.Profiles = profileTelemetryFixture(func(t *ProfileTelemetrySettings) { t.Token = "" })
		}},
		{"profiles literal token", func(s *Settings) {
			s.Telemetry.Profiles = profileTelemetryFixture(func(t *ProfileTelemetrySettings) { t.Token = "glc_secret" })
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultSettings()
			tt.edit(&cfg)
			require.Error(t, cfg.Validate())
		})
	}
}

func telemetryFixture(edit func(*TelemetrySettings)) TelemetrySettings {
	t := TelemetrySettings{
		Enabled:    true,
		Endpoint:   "https://otlp-gateway-prod-us-central-0.grafana.net/otlp",
		InstanceID: "123456",
		Token:      "env:HIVE_GRAFANACLOUD_TOKEN",
	}
	edit(&t)
	return t
}

func profileTelemetryFixture(edit func(*ProfileTelemetrySettings)) ProfileTelemetrySettings {
	t := ProfileTelemetrySettings{
		Enabled:  true,
		Endpoint: "https://profiles-prod-us-central-0.grafana.net",
		User:     "123456",
		Token:    "env:HIVE_GRAFANACLOUD_PROFILES_TOKEN",
	}
	edit(&t)
	return t
}

// A telemetry endpoint is deliberately remote, unlike every other URL setting.
// The loopback rule that guards development.github.api_base must not creep
// onto it.
func TestTelemetryAcceptsARemoteEndpoint(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Telemetry = telemetryFixture(func(*TelemetrySettings) {})
	require.NoError(t, cfg.Validate())
}

func TestTelemetryAcceptsEverySecretReferenceForm(t *testing.T) {
	for _, ref := range []string{
		"env:HIVE_GRAFANACLOUD_TOKEN",
		"file:/etc/hive/otlp-token",
		"op://Private/Grafana Cloud/credential",
	} {
		t.Run(ref, func(t *testing.T) {
			cfg := DefaultSettings()
			cfg.Telemetry = telemetryFixture(func(s *TelemetrySettings) { s.Token = ref })
			require.NoError(t, cfg.Validate())
		})
	}
}

// The whole destination can live in one secret, so the endpoint and the
// instance id take a reference too. Unlike the token they may also be written
// out, which is why a literal is still checked for shape.
func TestTelemetryEndpointAndInstanceIDAcceptReferences(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Telemetry = telemetryFixture(func(s *TelemetrySettings) {
		s.Endpoint = "op://Private/Grafana Cloud/endpoint"
		s.InstanceID = "op://Private/Grafana Cloud/username"
	})
	require.NoError(t, cfg.Validate())
}

// A reference's target is unknown until launch, so the https rule cannot be
// applied to it here. The telemetry package checks the resolved value.
func TestTelemetryDefersURLChecksOnAReference(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Telemetry = telemetryFixture(func(s *TelemetrySettings) { s.Endpoint = "file:/etc/hive/otlp-endpoint" })
	require.NoError(t, cfg.Validate())
}

// Nothing is required while the section is off, so a half-filled block does
// not stop the app from starting.
func TestTelemetryDisabledSkipsValidation(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Telemetry = TelemetrySettings{Enabled: false, Endpoint: "http://not-a-real-endpoint", Token: "pasted-literal"}
	require.NoError(t, cfg.Validate())
}

func TestProfileTelemetryIsIndependentOfOTLP(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Telemetry.Profiles = profileTelemetryFixture(func(*ProfileTelemetrySettings) {})
	require.NoError(t, cfg.Validate())
}

func TestProfileTelemetryAcceptsReferences(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Telemetry.Profiles = profileTelemetryFixture(func(s *ProfileTelemetrySettings) {
		s.Endpoint = "op://Private/Grafana Cloud/profiles-endpoint"
		s.User = "op://Private/Grafana Cloud/profiles-user"
		s.Token = "op://Private/Grafana Cloud/profiles-token"
	})
	require.NoError(t, cfg.Validate())
}

func TestProfileTelemetryDisabledSkipsValidation(t *testing.T) {
	cfg := DefaultSettings()
	cfg.Telemetry.Profiles = ProfileTelemetrySettings{
		Endpoint: "http://not-a-real-endpoint",
		Token:    "pasted-literal",
	}
	require.NoError(t, cfg.Validate())
}

func TestTelemetryHostIDEnvironmentOverride(t *testing.T) {
	path := isolateSettings(t)
	t.Setenv("HIVE_DESKTOP_TELEMETRY_HOST_ID", "machine-a")

	cfg, err := NewStore(path).Effective()
	require.NoError(t, err)
	assert.Equal(t, "machine-a", cfg.Telemetry.HostID)
}

func TestProfileTelemetryEnvironmentOverrides(t *testing.T) {
	path := isolateSettings(t)
	t.Setenv("HIVE_DESKTOP_TELEMETRY_PROFILES_ENABLED", "true")
	t.Setenv("HIVE_DESKTOP_TELEMETRY_PROFILES_ENDPOINT", "https://profiles.example.com")
	t.Setenv("HIVE_DESKTOP_TELEMETRY_PROFILES_USER", "profiles-user")
	t.Setenv("HIVE_DESKTOP_TELEMETRY_PROFILES_TOKEN", "env:PROFILES_TOKEN")

	cfg, err := NewStore(path).Effective()
	require.NoError(t, err)
	assert.Equal(t, ProfileTelemetrySettings{
		Enabled:  true,
		Endpoint: "https://profiles.example.com",
		User:     "profiles-user",
		Token:    "env:PROFILES_TOKEN",
	}, cfg.Telemetry.Profiles)
}

// A struct tag cannot reference a constant, so the env name is written twice.
// EnvironmentOverridden lookups key off the constant while the parser keys off
// the tag, and a silent drift between them would report "not overridden" for a
// value that was in fact overridden.
func TestEnvGitHubAPIBaseMatchesStructTag(t *testing.T) {
	field, ok := reflect.TypeFor[GitHubDevSettings]().FieldByName("APIBase")
	require.True(t, ok)
	assert.Equal(t, EnvGitHubAPIBase, field.Tag.Get("env"))
}

func TestGitHubAPIBaseAcceptsLoopbackAndNormalizes(t *testing.T) {
	tests := []struct {
		name string
		set  string
		want string
	}{
		{"unset means api.github.com", "", ""},
		{"loopback ip", "http://127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"localhost", "http://localhost:8080", "http://localhost:8080"},
		{"ipv6 loopback", "http://[::1]:8080", "http://[::1]:8080"},
		{"trailing slash trimmed", "http://127.0.0.1:8080/", "http://127.0.0.1:8080"},
		{"surrounding space trimmed", "  http://127.0.0.1:8080  ", "http://127.0.0.1:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultSettings()
			cfg.Development.GitHub.APIBase = tt.set
			require.NoError(t, cfg.Validate())
			assert.Equal(t, tt.want, cfg.GitHubAPIBase())
		})
	}
}

// The override is loopback-only wherever it comes from: being a persisted
// setting must not make it a way to aim a shipped app at a remote host.
func TestGitHubAPIBaseRejectsRemoteHostFromEnvironment(t *testing.T) {
	t.Setenv(EnvGitHubAPIBase, "https://api.github.example.com")
	_, err := NewStore(isolateSettings(t)).Effective()
	require.Error(t, err)
}

func TestGitHubAPIBaseEnvironmentOverrideIsNotPersisted(t *testing.T) {
	store := NewStore(isolateSettings(t))
	t.Setenv(EnvGitHubAPIBase, "http://127.0.0.1:9999")

	effective, err := store.Update(func(cfg *Settings) error {
		cfg.Appearance.Theme = "dark"
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:9999", effective.GitHubAPIBase())
	assert.True(t, effective.EnvironmentOverridden(EnvGitHubAPIBase))

	persisted, err := store.Persisted()
	require.NoError(t, err)
	assert.Empty(t, persisted.GitHubAPIBase())
}

func TestStoreUpdateReappliesEnvironmentWithoutMaterializingIt(t *testing.T) {
	store := NewStore(isolateSettings(t))
	t.Setenv("HIVE_DESKTOP_NOTIFICATIONS_ENABLED", "false")

	effective, err := store.Update(func(cfg *Settings) error {
		cfg.Appearance.Theme = "dark"
		cfg.Notifications.Enabled = true
		return nil
	})
	require.NoError(t, err)
	assert.False(t, effective.Notifications.Enabled)

	persisted, err := store.Persisted()
	require.NoError(t, err)
	assert.True(t, persisted.Notifications.Enabled)
	assert.Equal(t, "dark", persisted.Appearance.Theme)
}

func TestStoreSerializesConcurrentMutations(t *testing.T) {
	store := NewStore(isolateSettings(t))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := store.Update(func(cfg *Settings) error {
			cfg.Appearance.Theme = "dark"
			return nil
		})
		assert.NoError(t, err)
	}()
	go func() {
		defer wg.Done()
		_, err := store.Update(func(cfg *Settings) error {
			cfg.HTTP.Port = 24567
			return nil
		})
		assert.NoError(t, err)
	}()
	wg.Wait()

	persisted, err := store.Persisted()
	require.NoError(t, err)
	assert.Equal(t, "dark", persisted.Appearance.Theme)
	assert.Equal(t, 24567, persisted.HTTP.Port)
}
