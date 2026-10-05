package settings

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
	"github.com/colonyops/hive/internal/platform/secrets"
)

const settingsFileName = "settings.yaml"

const (
	MinPollInterval = 60 * time.Second
	// MaxDebugPause prevents a stale development setting from making startup
	// appear permanently hung while still allowing deliberate crash-window tests.
	MaxDebugPause = 10 * time.Minute
)

const (
	WebhookPortMin = 20000
	WebhookPortMax = 32767
)

var reservedWebhookPorts = map[int]bool{
	20000: true, 22000: true, 24800: true, 25565: true, 26257: true,
	27017: true, 27018: true, 27019: true, 28017: true, 29418: true,
	31337: true, 32400: true,
}

const (
	ChannelStable = "stable"
	ChannelBeta   = "beta"
	ChannelDev    = "dev"

	DeliveryAuto   = "auto"
	DeliverySystem = "system"
	DeliveryApp    = "app"

	MockLive        = "live"
	MockFeed        = "feed"
	MockPipeline    = "pipeline"
	MockOnboarding  = "onboarding"
	MockActionSmoke = "action-smoke"
)

// Duration is a YAML- and environment-friendly Go duration.
type Duration time.Duration

func (d Duration) Duration() time.Duration { return time.Duration(d) }
func (d Duration) String() string          { return time.Duration(d).String() }

func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

type PollingSettings struct {
	Interval Duration `yaml:"interval" env:"HIVE_DESKTOP_POLLING_INTERVAL"`
}

type UpdateSettings struct {
	Enabled bool   `yaml:"enabled"           env:"HIVE_DESKTOP_UPDATES_ENABLED"`
	Channel string `yaml:"channel,omitempty" env:"HIVE_DESKTOP_UPDATES_CHANNEL"`
}

type NotificationSettings struct {
	Enabled  bool   `yaml:"enabled"  env:"HIVE_DESKTOP_NOTIFICATIONS_ENABLED"`
	Delivery string `yaml:"delivery" env:"HIVE_DESKTOP_NOTIFICATIONS_DELIVERY"`
	Sound    bool   `yaml:"sound"    env:"HIVE_DESKTOP_NOTIFICATIONS_SOUND"`
}

type Appearance struct {
	Theme string `yaml:"theme,omitempty" env:"HIVE_DESKTOP_APPEARANCE_THEME"`
	// FontFamily names the family the app's chrome draws with and
	// MonoFontFamily the one its monospace text draws with, neither of which
	// touches a terminal — that has its own family below. Empty is the bundled
	// face; a CSS generic keyword (system-ui, ui-monospace) is the platform
	// stack.
	FontFamily     string `yaml:"font_family,omitempty"      env:"HIVE_DESKTOP_APPEARANCE_FONT_FAMILY"`
	MonoFontFamily string `yaml:"mono_font_family,omitempty" env:"HIVE_DESKTOP_APPEARANCE_MONO_FONT_FAMILY"`
	// Pixels; 0 is the shipped default.
	TerminalFontSize int `yaml:"terminal_font_size,omitempty" env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_FONT_SIZE"`
	// TerminalFontFamily names an installed monospace family for the terminal
	// only, leaving the rest of the UI alone. Empty is the bundled face.
	TerminalFontFamily string `yaml:"terminal_font_family,omitempty" env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_FONT_FAMILY"`
	// TerminalFontWeight is the CSS weight normal cells draw at, and
	// TerminalFontWeightBold the weight a bold cell draws at. Zero means
	// nothing persisted; the frontend owns the defaults and the valid set.
	TerminalFontWeight     int `yaml:"terminal_font_weight,omitempty"      env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_FONT_WEIGHT"`
	TerminalFontWeightBold int `yaml:"terminal_font_weight_bold,omitempty" env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_FONT_WEIGHT_BOLD"`
	// TerminalLineHeight multiplies the cell height and TerminalLetterSpacing
	// widens the cell by whole device pixels. Zero means nothing persisted, so
	// the letter-spacing default has to stay zero — moving it would make "no
	// extra tracking" unselectable.
	TerminalLineHeight    float64 `yaml:"terminal_line_height,omitempty"    env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_LINE_HEIGHT"`
	TerminalLetterSpacing int     `yaml:"terminal_letter_spacing,omitempty" env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_LETTER_SPACING"`
	// TerminalShowWindows lists every active session's tmux windows in the
	// terminal sidebar, not just the attached session's. On by default.
	TerminalShowWindows bool `yaml:"terminal_show_windows" env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_SHOW_WINDOWS"`
	// TerminalShowStatusBar gives the attached session the same status bar a
	// chat has, plus git and pull-request state. On by default.
	TerminalShowStatusBar bool `yaml:"terminal_show_status_bar" env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_SHOW_STATUS_BAR"`
	// TerminalPoolSize is how many sessions the terminal view keeps attached at
	// once for instant switching (ADR terminal-attach-pool). Like the other appearance values it
	// is carried verbatim and healed by the frontend: anything outside 1-6 reads
	// as the default, 3.
	TerminalPoolSize int `yaml:"terminal_pool_size" env:"HIVE_DESKTOP_APPEARANCE_TERMINAL_POOL_SIZE"`
	// CanvasFontSize, CanvasLineSpacing and CanvasPageWidth are preset names
	// interpreted by the frontend. Empty takes the shipped defaults.
	CanvasFontSize    string `yaml:"canvas_font_size,omitempty"    env:"HIVE_DESKTOP_APPEARANCE_CANVAS_FONT_SIZE"`
	CanvasLineSpacing string `yaml:"canvas_line_spacing,omitempty" env:"HIVE_DESKTOP_APPEARANCE_CANVAS_LINE_SPACING"`
	CanvasPageWidth   string `yaml:"canvas_page_width,omitempty"   env:"HIVE_DESKTOP_APPEARANCE_CANVAS_PAGE_WIDTH"`
}

// ProfileSettings configures the profile rail. Order names flow ids in the
// order they should appear; an id it omits sorts alphabetically after every id
// it names, so the list never has to be exhaustive and a newly created profile
// lands at the end rather than somewhere unspecified. An id naming no flow is
// ignored, so deleting a profile does not invalidate the setting.
type ProfileSettings struct {
	Order []string `yaml:"order,omitempty"`
}

const (
	// MaxMenuBarFeeds keeps the menu bar dropdown short enough to scan; the
	// full inbox is the main window's job.
	MaxMenuBarFeeds         = 3
	DefaultMenuBarItemLimit = 3
	MaxMenuBarItemLimit     = 10
)

// MenuBarSettings preserves feed order and permits missing feeds.
type MenuBarSettings struct {
	Feeds []MenuBarFeed `yaml:"feeds,omitempty"`
}

type MenuBarFeed struct {
	// Feed is a feed id, "<flow id>/<feed node id>".
	Feed string `yaml:"feed"`
	// Zero uses DefaultMenuBarItemLimit.
	Limit int `yaml:"limit,omitempty"`
}

func (f MenuBarFeed) ItemLimit() int {
	if f.Limit == 0 {
		return DefaultMenuBarItemLimit
	}
	return f.Limit
}

func (m MenuBarSettings) Validate() error {
	if len(m.Feeds) > MaxMenuBarFeeds {
		return fmt.Errorf("menu_bar.feeds allows at most %d feeds", MaxMenuBarFeeds)
	}
	seen := make(map[string]bool, len(m.Feeds))
	for _, pin := range m.Feeds {
		profile, node, ok := strings.Cut(pin.Feed, "/")
		if !ok || profile == "" || node == "" {
			return fmt.Errorf("menu_bar.feeds: %q is not a feed id of the form <profile>/<feed>", pin.Feed)
		}
		if seen[pin.Feed] {
			return fmt.Errorf("menu_bar.feeds: %q is listed twice", pin.Feed)
		}
		seen[pin.Feed] = true
		if pin.Limit < 0 || pin.Limit > MaxMenuBarItemLimit {
			return fmt.Errorf("menu_bar.feeds: %q limit must be between 1 and %d, or omitted for the default", pin.Feed, MaxMenuBarItemLimit)
		}
	}
	return nil
}

// AgentWorkspacesSettings locates the agent-workspace root. Empty resolves to
// <ConfigDir>/workspaces; a leading `~` is expanded at read time. It is
// configurable because iCloud Drive is an expected destination (spec §4.4).
type AgentWorkspacesSettings struct {
	Dir string `yaml:"dir,omitempty" env:"HIVE_DESKTOP_AGENT_WORKSPACES_DIR"`
	// SessionEndDelay is the grace between a chat asking to end its own
	// session and the session being ended: the request arrives from inside the
	// agent's own tool call, and the delay lets that call return first. Zero
	// takes the shipped value.
	SessionEndDelay Duration `yaml:"session_end_delay,omitempty" env:"HIVE_DESKTOP_AGENT_WORKSPACES_SESSION_END_DELAY"`
}

// PathsSettings locates the external binaries the app execs. Each is the escape
// hatch for an install discovery does not know about (ADR tmux-discovery): empty — the
// shipped value — searches PATH and the usual package-manager prefixes.
type PathsSettings struct {
	Tmux string `yaml:"tmux,omitempty" env:"HIVE_DESKTOP_PATHS_TMUX"`
}

// EditorSettings names the editor "Open in editor" actions launch on a
// directory. Command is a single word — a CLI launcher name (zed, code) or an
// absolute path — never a command line: the same rule agent commands follow
// (ADR a-workspace-declares-its-own-authority), so a flag cannot ride in through a settings string. Empty means
// none configured.
type EditorSettings struct {
	Command string `yaml:"command,omitempty" env:"HIVE_DESKTOP_EDITOR_COMMAND"`
}

// OnboardingSettings records that first run was walked to its end. It is the
// one thing first run cannot infer: every other step has its own signal (a
// usable Hive config, a connected account, a resolved notification grant),
// but the closing hand-off to the agent leaves nothing behind to read.
type OnboardingSettings struct {
	Completed bool `yaml:"completed,omitempty"`
}

// TelemetrySettings configures OTLP export of the app's own signals. It is
// top-level rather than under development because it is the user's own
// observability, not a debug facility.
//
// Endpoint is the signal-less OTLP base; on Grafana Cloud InstanceID is the
// OTLP instance id from the stack's OpenTelemetry tile, not the stack id.
// HostID becomes the OpenTelemetry host.id resource attribute. It is optional
// because it is a stable machine identifier the user must choose to disclose
// (ADR telemetry-carries-the-opentelemetry-host-identifier-configured-for-the-machine).
//
// Endpoint, InstanceID, and Token may be a internal/platform/secrets reference — "env:NAME",
// "file:/path", "op://vault/item/field" — so one 1Password item can hold a
// whole destination. They differ in whether a literal is allowed: Token
// *requires* a reference, because a literal there is a credential in a
// dotfiles-managed file, while Endpoint and InstanceID name a destination and
// are ordinarily written out.
//
// A reference is resolved at launch, so Validate checks Endpoint's URL shape
// only when it is written out. The resolved value is checked either way, by
// the telemetry package.
type TelemetrySettings struct {
	Enabled    bool                     `yaml:"enabled"               env:"HIVE_DESKTOP_TELEMETRY_ENABLED"`
	Endpoint   string                   `yaml:"endpoint,omitempty"    env:"HIVE_DESKTOP_TELEMETRY_ENDPOINT"`
	InstanceID string                   `yaml:"instance_id,omitempty" env:"HIVE_DESKTOP_TELEMETRY_INSTANCE_ID"`
	HostID     string                   `yaml:"host_id,omitempty"     env:"HIVE_DESKTOP_TELEMETRY_HOST_ID"`
	Token      string                   `yaml:"token,omitempty"       env:"HIVE_DESKTOP_TELEMETRY_TOKEN"`
	Profiles   ProfileTelemetrySettings `yaml:"profiles,omitempty"`
}

// ProfileTelemetrySettings configures direct Pyroscope export. Grafana Cloud
// Profiles has its own endpoint and basic-auth user, independent of OTLP.
type ProfileTelemetrySettings struct {
	Enabled  bool   `yaml:"enabled"            env:"HIVE_DESKTOP_TELEMETRY_PROFILES_ENABLED"`
	Endpoint string `yaml:"endpoint,omitempty" env:"HIVE_DESKTOP_TELEMETRY_PROFILES_ENDPOINT"`
	User     string `yaml:"user,omitempty"     env:"HIVE_DESKTOP_TELEMETRY_PROFILES_USER"`
	Token    string `yaml:"token,omitempty"    env:"HIVE_DESKTOP_TELEMETRY_PROFILES_TOKEN"`
}

// HTTPSettings configures the local loopback HTTP server that hosts both the
// webhook listener and the agent API. On by default: it is loopback-only, so it
// is reachable only from this machine.
type HTTPSettings struct {
	Enabled bool   `yaml:"enabled" env:"HIVE_DESKTOP_HTTP_ENABLED"`
	Host    string `yaml:"host"    env:"HIVE_DESKTOP_HTTP_HOST"`
	Port    int    `yaml:"port"    env:"HIVE_DESKTOP_HTTP_PORT"`
}

type MockSettings struct {
	Mode string `yaml:"mode" env:"HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE"`
}

type ServerSettings struct {
	Host string `yaml:"host" env:"HOST"`
	Port int    `yaml:"port" env:"PORT"`
}

// PprofSettings gates the pprof endpoint; when enabled it mounts on the shared
// HTTP server (ADR pprof-debug-endpoint), so it has no host/port of its own.
type PprofSettings struct {
	Enabled bool `yaml:"enabled" env:"HIVE_DESKTOP_DEVELOPMENT_PPROF_ENABLED"`
}

// PerfSettings gates the UI performance recorder, which appends spans the
// frontend emits to perf.jsonl under the state directory (ADR ui-performance-spans-are-recorded-to-jsonl). Off in a
// shipped build; the dev task turns it on through launch.env.
type PerfSettings struct {
	Enabled bool `yaml:"enabled" env:"HIVE_DESKTOP_DEVELOPMENT_PERF_ENABLED"`
}

// MetricsSettings gates the local Prometheus scrape endpoint, mounted on the
// shared HTTP server the way pprof is (ADR pprof-debug-endpoint). Independent of telemetry.enabled:
// the same instruments feed both readers.
type MetricsSettings struct {
	Enabled bool `yaml:"enabled" env:"HIVE_DESKTOP_DEVELOPMENT_METRICS_ENABLED"`
}

// DevToolsSettings makes the in-app developer tools reachable in a build that
// was not served by Vite (ADR developer-tools-are-reachable-in-a-shipped-build-behind-a-setting). Off in a shipped build; the dev task
// turns it on through launch.env.
type DevToolsSettings struct {
	Enabled bool `yaml:"enabled" env:"HIVE_DESKTOP_DEVELOPMENT_DEVTOOLS_ENABLED"`
}

type DebugSettings struct {
	PauseIngest Duration `yaml:"pause_ingest" env:"HIVE_DESKTOP_DEVELOPMENT_DEBUG_PAUSE_INGEST"`
	PauseCommit Duration `yaml:"pause_commit" env:"HIVE_DESKTOP_DEVELOPMENT_DEBUG_PAUSE_COMMIT"`
}

// EnvGitHubAPIBase is the environment name behind development.github.api_base.
// It is named here because startup reports whether the value it is running
// with came from the environment or from settings.yaml.
const EnvGitHubAPIBase = "HIVE_DESKTOP_DEVELOPMENT_GITHUB_API_BASE"

// GitHubDevSettings redirects the GitHub REST/GraphQL base at cmd/desktop/devserver,
// the development caching proxy and event simulator (ADR devserver-github-proxy). Empty — the
// shipped value — means api.github.com.
//
// Only the API base moves. The OAuth base stays github.com: a device-flow
// token exchange has no business passing through dev tooling, and it draws no
// rate-limit budget, so redirecting it would be all risk and no benefit.
type GitHubDevSettings struct {
	APIBase string `yaml:"api_base,omitempty" env:"HIVE_DESKTOP_DEVELOPMENT_GITHUB_API_BASE"`
}

type DevelopmentSettings struct {
	Mocks    MockSettings      `yaml:"mocks"`
	GitHub   GitHubDevSettings `yaml:"github,omitempty"`
	Vite     ServerSettings    `yaml:"vite"             envPrefix:"HIVE_DESKTOP_DEVELOPMENT_VITE_"`
	Wails    ServerSettings    `yaml:"wails"            envPrefix:"HIVE_DESKTOP_DEVELOPMENT_WAILS_"`
	Pprof    PprofSettings     `yaml:"pprof"`
	Perf     PerfSettings      `yaml:"perf"`
	Metrics  MetricsSettings   `yaml:"metrics"`
	DevTools DevToolsSettings  `yaml:"devtools"`
	Debug    DebugSettings     `yaml:"debug"`
}

// Settings is the typed settings.yaml schema. Environment override provenance
// is process-local and is never serialized.
type Settings struct {
	Version       int                  `yaml:"version"`
	Polling       PollingSettings      `yaml:"polling"`
	Updates       UpdateSettings       `yaml:"updates"`
	Notifications NotificationSettings `yaml:"notifications"`
	// No omitempty: with terminal_show_windows off and nothing else set the
	// struct is all-zero, and an omitted section would read back as defaults.
	Appearance      Appearance              `yaml:"appearance"`
	Profiles        ProfileSettings         `yaml:"profiles,omitempty"`
	MenuBar         MenuBarSettings         `yaml:"menu_bar,omitempty"`
	HTTP            HTTPSettings            `yaml:"http"`
	Telemetry       TelemetrySettings       `yaml:"telemetry"`
	Keybindings     map[string][]string     `yaml:"keybindings,omitempty"`
	Paths           PathsSettings           `yaml:"paths,omitempty"`
	Editor          EditorSettings          `yaml:"editor,omitempty"`
	AgentWorkspaces AgentWorkspacesSettings `yaml:"agent_workspaces,omitempty"`
	Onboarding      OnboardingSettings      `yaml:"onboarding,omitempty"`
	Development     DevelopmentSettings     `yaml:"development"`

	overrides   map[string]bool
	unknownKeys error
}

func DefaultSettings() Settings {
	return Settings{
		Version:         configmigrate.SettingsSet.Current,
		Polling:         PollingSettings{Interval: Duration(5 * time.Minute)},
		Updates:         UpdateSettings{Enabled: true},
		Notifications:   NotificationSettings{Enabled: true, Delivery: DeliveryAuto, Sound: true},
		Appearance:      Appearance{TerminalShowWindows: true, TerminalShowStatusBar: true, TerminalPoolSize: 3},
		HTTP:            HTTPSettings{Enabled: true, Host: "127.0.0.1", Port: 0},
		Telemetry:       TelemetrySettings{Enabled: false},
		AgentWorkspaces: AgentWorkspacesSettings{SessionEndDelay: Duration(10 * time.Second)},
		Development: DevelopmentSettings{
			Mocks:    MockSettings{Mode: MockLive},
			Vite:     ServerSettings{Host: "127.0.0.1", Port: 0},
			Wails:    ServerSettings{Host: "127.0.0.1", Port: 0},
			Pprof:    PprofSettings{Enabled: false},
			Perf:     PerfSettings{Enabled: false},
			Metrics:  MetricsSettings{Enabled: false},
			DevTools: DevToolsSettings{Enabled: false},
		},
	}
}

func (s Settings) EnvironmentOverridden(name string) bool { return s.overrides[name] }

// UnknownKeys reports the settings.yaml keys this build does not declare, one
// line each, or nil. The load ignores them and a save drops them.
func (s Settings) UnknownKeys() error { return s.unknownKeys }

func (s Settings) MockMode() string {
	if s.Development.Mocks.Mode == MockLive {
		return ""
	}
	return s.Development.Mocks.Mode
}

// GitHubAPIBase returns the canonical API base override, or "" for the
// client's own api.github.com default. The trailing slash is trimmed because
// the client concatenates paths onto this value directly.
func (s Settings) GitHubAPIBase() string {
	return normalizeAPIBase(s.Development.GitHub.APIBase)
}

func normalizeAPIBase(base string) string {
	return strings.TrimSuffix(strings.TrimSpace(base), "/")
}

func ResolveNotificationDelivery(value string) string {
	switch value {
	case DeliveryAuto, DeliverySystem, DeliveryApp:
		return value
	default:
		return DeliveryAuto
	}
}

func (s Settings) Validate() error {
	if s.Polling.Interval.Duration() < MinPollInterval {
		return fmt.Errorf("polling.interval must be at least %s", MinPollInterval)
	}
	switch s.Updates.Channel {
	case "", ChannelStable, ChannelBeta, ChannelDev:
	default:
		return fmt.Errorf("updates.channel must be stable, beta, or dev")
	}
	switch s.Notifications.Delivery {
	case DeliveryAuto, DeliverySystem, DeliveryApp:
	default:
		return fmt.Errorf("notifications.delivery must be auto, system, or app")
	}
	if !validListenerHost(s.HTTP.Host) {
		return fmt.Errorf("http.host must be a loopback address")
	}
	if !ValidListenerPort(s.HTTP.Port) {
		return fmt.Errorf("http.port must be 0 or between 1024 and 65535")
	}
	if s.Paths.Tmux != "" && !filepath.IsAbs(s.Paths.Tmux) {
		return fmt.Errorf("paths.tmux must be an absolute path")
	}
	if err := s.MenuBar.Validate(); err != nil {
		return err
	}
	if len(strings.Fields(s.Editor.Command)) > 1 {
		return fmt.Errorf("editor.command must be a single word — a command name or path, without flags")
	}
	switch s.Development.Mocks.Mode {
	case MockLive, MockFeed, MockPipeline, MockOnboarding, MockActionSmoke:
	default:
		return fmt.Errorf("development.mocks.mode is invalid")
	}
	// Wails constructs the frontend development URL with localhost rather than
	// the configured Vite host. Binding Vite to another loopback address (for
	// example 127.0.0.2) therefore starts successfully but leaves Wails unable
	// to reach it. Keep the known-working explicit IPv4 loopback until Wails can
	// consume the host as part of that URL.
	if s.Development.Vite.Host != "127.0.0.1" || !ValidListenerPort(s.Development.Vite.Port) {
		return fmt.Errorf("development.vite.host must be 127.0.0.1 and port must be 0 or between 1024 and 65535")
	}
	if !validServer(s.Development.Wails) {
		return fmt.Errorf("development.wails must use a loopback host and port must be 0 or between 1024 and 65535")
	}
	if s.Development.Debug.PauseIngest < 0 || s.Development.Debug.PauseCommit < 0 {
		return fmt.Errorf("development debug pauses must not be negative")
	}
	if s.Development.Debug.PauseIngest.Duration() > MaxDebugPause || s.Development.Debug.PauseCommit.Duration() > MaxDebugPause {
		return fmt.Errorf("development debug pauses must not exceed %s", MaxDebugPause)
	}
	if err := validateTelemetry(s.Telemetry); err != nil {
		return err
	}
	// Loopback-only, for the same reason the webhook listener is (ADR local-webhook-listener):
	// this value redirects an authenticated GitHub client, so the only host
	// allowed to receive that traffic is one on this machine. Because it is
	// enforced here, it holds for a value arriving from settings.yaml and from
	// the environment alike — a persisted setting cannot aim the app at a
	// remote collector.
	if err := validateGitHubAPIBase(s.GitHubAPIBase()); err != nil {
		return err
	}
	return nil
}

// validateTelemetry deliberately does not apply validateGitHubAPIBase's
// loopback rule. Telemetry destinations are remote, so HTTPS stops credentials
// from crossing the network in the clear. Resolved values are checked again by
// the telemetry package at launch.
func validateTelemetry(t TelemetrySettings) error {
	if t.Enabled {
		if err := validateTelemetryDestination("telemetry", "instance_id", t.Endpoint, t.InstanceID, t.Token); err != nil {
			return err
		}
	}
	if t.Profiles.Enabled {
		if err := validateTelemetryDestination("telemetry.profiles", "user", t.Profiles.Endpoint, t.Profiles.User, t.Profiles.Token); err != nil {
			return err
		}
	}
	return nil
}

func validateTelemetryDestination(section, userField, endpointValue, userValue, token string) error {
	endpoint := strings.TrimSpace(endpointValue)
	if endpoint == "" {
		return fmt.Errorf("%s.endpoint is required when %s.enabled is true", section, section)
	}
	// A reference's target is unknown until launch, so only a written-out
	// endpoint can be checked here. Resolving during Validate would shell out
	// to a secret manager on every settings save.
	if !secrets.HasKnownPrefix(endpoint) {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return fmt.Errorf("%s.endpoint must be a valid URL: %w", section, err)
		}
		if parsed.Scheme != "https" {
			return fmt.Errorf("%s.endpoint must use https", section)
		}
		if parsed.Host == "" {
			return fmt.Errorf("%s.endpoint must include a host", section)
		}
	}
	if strings.TrimSpace(userValue) == "" {
		return fmt.Errorf("%s.%s is required when %s.enabled is true", section, userField, section)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%s.token is required when %s.enabled is true", section, section)
	}
	if !secrets.HasKnownPrefix(token) {
		return fmt.Errorf("%s.token must be a reference (env:NAME, file:/path, or op://vault/item/field), not a literal secret", section)
	}
	return nil
}

func validateGitHubAPIBase(base string) error {
	if base == "" {
		return nil
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("development.github.api_base must be a valid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("development.github.api_base must use http or https")
	}
	if !validListenerHost(parsed.Hostname()) {
		return fmt.Errorf("development.github.api_base must point at a loopback host")
	}
	return nil
}

func validServer(server ServerSettings) bool {
	return validListenerHost(server.Host) && ValidListenerPort(server.Port)
}

func validListenerHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ValidListenerPort(port int) bool { return port == 0 || (port >= 1024 && port <= 65535) }

// Package-level helpers remain for isolated tests and e2e harnesses. Runtime
// composition owns and injects one Store at its resolved Paths.SettingsPath.
func LoadPersistedSettings() (Settings, error) { return NewStore(SettingsPath()).Persisted() }
func LoadSettings() (Settings, error)          { return NewStore(SettingsPath()).Effective() }

func SaveSettings(cfg Settings) error {
	settingsFileMu.Lock()
	defer settingsFileMu.Unlock()
	return saveSettingsAt(SettingsPath(), cfg)
}

// AllocateWebhookPort offers a stable candidate for the settings UI. Runtime
// automatic allocation binds port zero directly and does not use this probe.
func AllocateWebhookPort(ctx context.Context) (int, error) {
	var lc net.ListenConfig
	const attempts = 64
	span := WebhookPortMax - WebhookPortMin + 1
	for range attempts {
		port := WebhookPortMin + rand.IntN(span)
		if reservedWebhookPorts[port] {
			continue
		}
		ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no free port found in %d-%d after %d attempts", WebhookPortMin, WebhookPortMax, attempts)
}
