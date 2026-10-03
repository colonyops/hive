// Package settings resolves the desktop app's typed settings and on-disk
// locations. Desktop-owned overrides use the HIVE_DESKTOP_ prefix; XDG and
// shared Hive inputs remain external boundaries.
package settings

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/colonyops/hive/pkg/pathutil"
)

const (
	EnvDataDir     = "HIVE_DESKTOP_DATA_DIR"
	EnvHiveDataDir = "HIVE_DESKTOP_HIVE_DATA_DIR"
	EnvConfigDir   = "HIVE_DESKTOP_CONFIG_DIR"
	EnvFlowsDir    = "HIVE_DESKTOP_FLOWS_DIR"
	EnvActionsPath = "HIVE_DESKTOP_ACTIONS_PATH"
	EnvMockMode    = "HIVE_DESKTOP_DEVELOPMENT_MOCKS_MODE"
	EnvE2EHarness  = "HIVE_DESKTOP_E2E_HARNESS"

	EnvHTTPEnabled = "HIVE_DESKTOP_HTTP_ENABLED"
	EnvHTTPPort    = "HIVE_DESKTOP_HTTP_PORT"

	// EnvAgentWorkspacesDir is the environment name behind
	// agent_workspaces.dir. Named here, duplicating the settings.go struct
	// tag, because a struct tag cannot reference a const and startup reports
	// provenance by name.
	EnvAgentWorkspacesDir = "HIVE_DESKTOP_AGENT_WORKSPACES_DIR"

	EnvPerfEnabled = "HIVE_DESKTOP_DEVELOPMENT_PERF_ENABLED"
)

// Paths is the immutable startup snapshot of every desktop-owned location.
// Runtime code receives this value instead of resolving process environment
// repeatedly.
type Paths struct {
	DataDir string
	// HiveDataDir holds hive.db, shared with the external hive CLI. It defaults
	// to DataDir; dev overrides it to the installed hive data dir so sessions
	// created in dev land in the real database while desktop state stays isolated.
	HiveDataDir string
	StateDir    string
	ConfigDir   string
	FlowsDir    string
	ActionsPath string
	// AgentWorkspacesDir is the agent-workspace root: agent_workspaces.dir,
	// resolved and `~`-expanded, or <ConfigDir>/workspaces when unset.
	AgentWorkspacesDir   string
	SettingsPath         string
	CredentialsIndexPath string
	LogFile              string
	// ReportsDir holds the diagnostic bundles "Report a problem" writes. It is
	// resolved here so the reporter and the system service's path allowlist
	// cannot disagree about where they live.
	ReportsDir          string
	DataDirOverridden   bool
	ConfigDirOverridden bool
}

// ResolveOptions carries the settings-derived inputs to path resolution. They
// are arguments rather than a post-hoc copy because Paths is a snapshot:
// every field is resolved once, by one function.
type ResolveOptions struct {
	// MockMode only affects the isolated onboarding flow path.
	MockMode string
	// AgentWorkspacesDir is settings' agent_workspaces.dir. Empty resolves to
	// <ConfigDir>/workspaces; a leading `~` is expanded here.
	AgentWorkspacesDir string
}

// ResolvePaths applies explicit environment overrides over bootstrap and
// settings values, then XDG defaults.
func ResolvePaths(b Bootstrap, opts ResolveOptions) Paths {
	dataDir, dataEnv := os.LookupEnv(EnvDataDir)
	dataOverride := dataEnv && dataDir != ""
	if !dataOverride {
		dataDir = b.DataDir
		if dataDir == "" {
			dataDir = filepath.Join(pathutil.XDGDataHome(), "hive")
		}
	}

	configDir, configEnv := os.LookupEnv(EnvConfigDir)
	configOverride := configEnv && configDir != ""
	if !configOverride {
		configDir = b.ConfigDir
		if configDir == "" {
			configDir = filepath.Join(pathutil.XDGConfigHome(), "hive", "desktop")
		}
	}

	hiveDataDir, hiveEnv := os.LookupEnv(EnvHiveDataDir)
	if !hiveEnv || hiveDataDir == "" {
		hiveDataDir = dataDir
	}

	stateDir := filepath.Join(dataDir, "desktop")
	flowsDir := os.Getenv(EnvFlowsDir)
	if flowsDir == "" {
		if opts.MockMode == MockOnboarding {
			flowsDir = onboardingFlowsDir()
		}
		if flowsDir == "" {
			flowsDir = filepath.Join(configDir, "flows")
		}
	}
	actionsPath := os.Getenv(EnvActionsPath)
	if actionsPath == "" {
		actionsPath = filepath.Join(configDir, "actions.yml")
	}

	agentWorkspacesDir, agentWorkspacesEnv := os.LookupEnv(EnvAgentWorkspacesDir)
	if !agentWorkspacesEnv || agentWorkspacesDir == "" {
		agentWorkspacesDir = opts.AgentWorkspacesDir
	}
	if agentWorkspacesDir == "" {
		agentWorkspacesDir = filepath.Join(configDir, "workspaces")
	} else {
		agentWorkspacesDir = pathutil.ExpandHome(agentWorkspacesDir)
	}

	return Paths{
		DataDir:              dataDir,
		HiveDataDir:          hiveDataDir,
		StateDir:             stateDir,
		ConfigDir:            configDir,
		FlowsDir:             flowsDir,
		ActionsPath:          actionsPath,
		AgentWorkspacesDir:   agentWorkspacesDir,
		SettingsPath:         filepath.Join(configDir, settingsFileName),
		CredentialsIndexPath: filepath.Join(stateDir, "credentials.json"),
		LogFile:              filepath.Join(stateDir, logFileName),
		ReportsDir:           filepath.Join(dataDir, "reports"),
		DataDirOverridden:    dataOverride || b.DataDir != "",
		ConfigDirOverridden:  configOverride || b.ConfigDir != "",
	}
}

func envMockMode() string {
	mode := os.Getenv(EnvMockMode)
	if mode == "" || mode == MockLive {
		return ""
	}
	return mode
}

func defaultPaths() Paths {
	b, _ := LoadBootstrap()
	return ResolvePaths(b, ResolveOptions{MockMode: envMockMode()})
}

// Package-level helpers are retained for isolated tests and e2e harnesses.
// Production runtime code uses the Paths snapshot injected from main.
func DataDir() string      { return defaultPaths().DataDir }
func StateDir() string     { return defaultPaths().StateDir }
func ConfigDir() string    { return defaultPaths().ConfigDir }
func FlowsDir() string     { return defaultPaths().FlowsDir }
func ActionsPath() string  { return defaultPaths().ActionsPath }
func SettingsPath() string { return defaultPaths().SettingsPath }
func ReportsDir() string   { return defaultPaths().ReportsDir }
func MockMode() string     { return envMockMode() }

var onboardingFlowsDir = sync.OnceValue(func() string {
	dir, err := os.MkdirTemp("", "hive-desktop-onboarding-")
	if err != nil {
		return ""
	}
	return dir
})
