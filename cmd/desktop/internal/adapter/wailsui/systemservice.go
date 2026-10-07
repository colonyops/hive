package wailsui

import (
	"context"
	"errors"
	"runtime"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SystemService exposes the app's on-disk locations to the System settings
// screen, plus the two things only a GUI can do: pick a directory natively
// and quit. Everything else — the path allowlist, the override validation
// with its write probe — is core logic behind app.SystemService.
type SystemService struct {
	system *app.SystemService
	// build is the running binary's version/commit/date. It is passed in
	// rather than read here because the ldflags that populate it bind to
	// package main (-X main.version), which cannot move to this package.
	build BuildInfo
}

// NewSystemService constructs the service over the core's system service and
// the running binary's build info.
func NewSystemService(system *app.SystemService, version, commit, date string) *SystemService {
	channel, _ := ReleaseChannel(version)
	return &SystemService{
		system: system,
		build: BuildInfo{
			Version:   version,
			Commit:    ShortCommit(commit),
			Date:      date,
			Channel:   channel,
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			GoVersion: runtime.Version(),
		},
	}
}

// PathInfo describes a single on-disk location surfaced in settings.
type PathInfo struct {
	Path string `json:"path"`
	// Exists reports whether the path is present on disk right now (a log file
	// or database may not exist until first written).
	Exists bool `json:"exists"`
	// Overridden reports whether an explicit override selected this location.
	Overridden bool `json:"overridden"`
}

// SystemInfo is the full set of locations the settings screens show.
type SystemInfo struct {
	DataDir     PathInfo `json:"dataDir"`
	ConfigDir   PathInfo `json:"configDir"`
	HiveDataDir PathInfo `json:"hiveDataDir"`
	LogFile     PathInfo `json:"logFile"`
	Database    PathInfo `json:"database"`
	HiveConfig  PathInfo `json:"hiveConfig"`
	// AgentWorkspaces is the agent-workspace root, shown on the Agents pane
	// rather than with the other locations: it is where the Agents area keeps
	// its workspaces, not part of the install.
	AgentWorkspaces PathInfo `json:"agentWorkspaces"`
}

// Info returns the effective locations for this running process plus whether
// the data/config directories are backed by a stored override.
func (s *SystemService) Info(ctx context.Context) SystemInfo {
	info := s.system.Info(ctx)
	return SystemInfo{
		DataDir:         pathInfo(info.DataDir),
		ConfigDir:       pathInfo(info.ConfigDir),
		HiveDataDir:     pathInfo(info.HiveDataDir),
		LogFile:         pathInfo(info.LogFile),
		Database:        pathInfo(info.Database),
		AgentWorkspaces: pathInfo(info.AgentWorkspaces),
		HiveConfig:      pathInfo(info.HiveConfig),
	}
}

func pathInfo(p app.PathInfo) PathInfo {
	return PathInfo{Path: p.Path, Exists: p.Exists, Overridden: p.Overridden}
}

// BuildInfo describes the running desktop build so users can see and report
// the exact version they are on from the System settings screen.
type BuildInfo struct {
	Version string `json:"version"`
	// Commit is the short (7-character) git revision the build was cut from.
	Commit string `json:"commit"`
	Date   string `json:"date"`
	// Channel is the release channel this version belongs to (stable, beta,
	// dev). Empty marks an unreleased build — which is also when the updater
	// engine is absent, so the About screen reads it as "self-update is off
	// for this build" rather than showing an update state it cannot reach.
	Channel string `json:"channel"`
	// OS and Arch are the Go build target this binary was compiled for, and
	// GoVersion the toolchain that compiled it — the same facts the problem
	// reporter attaches, shown so they can be read (and quoted) without
	// generating a bundle.
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"goVersion"`
}

// Build returns the version, commit, and date this desktop app was built from.
func (s *SystemService) Build() BuildInfo { return s.build }

// OpenPath opens one of the known system locations in the OS default
// application. The core validates the path against the current location set,
// so this RPC cannot be used to open arbitrary files.
func (s *SystemService) OpenPath(ctx context.Context, path string) error {
	return s.system.OpenPath(ctx, path)
}

// RevealPath reveals one of the known system locations in the OS file manager.
func (s *SystemService) RevealPath(ctx context.Context, path string) error {
	return s.system.RevealPath(ctx, path)
}

// OpenHiveConfig creates the resolved Hive config when it does not exist and
// opens it in the OS default application.
func (s *SystemService) OpenHiveConfig(ctx context.Context) error {
	return s.system.OpenHiveConfig(ctx)
}

// ChooseDirectory opens a native directory picker and returns the chosen path,
// or "" if the user cancels. GUI-only, so it stays here.
func (s *SystemService) ChooseDirectory(title string) (string, error) {
	wailsApp := application.Get()
	if wailsApp == nil {
		return "", errors.New("no application context for directory picker")
	}
	dialog := wailsApp.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true)
	if title != "" {
		dialog.SetTitle(title)
	}
	return dialog.PromptForSingleSelection()
}

// SetDataDir persists a data-directory override. It takes effect on the next
// launch; nothing is moved.
func (s *SystemService) SetDataDir(ctx context.Context, path string) error {
	return s.system.SetDataDir(ctx, path)
}

// SetConfigDir persists a config-directory override (flows, actions).
func (s *SystemService) SetConfigDir(ctx context.Context, path string) error {
	return s.system.SetConfigDir(ctx, path)
}

// ClearDataDir removes the data-directory override.
func (s *SystemService) ClearDataDir(ctx context.Context) error {
	return s.system.ClearDataDir(ctx)
}

// ClearConfigDir removes the config-directory override.
func (s *SystemService) ClearConfigDir(ctx context.Context) error {
	return s.system.ClearConfigDir(ctx)
}

// Quit terminates the app so the user can relaunch and apply a directory
// override in one click from the restart-required banner. GUI-only.
func (s *SystemService) Quit() {
	if wailsApp := application.Get(); wailsApp != nil {
		wailsApp.Quit()
	}
}
