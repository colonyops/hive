package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/desktop"
	"github.com/colonyops/hive/internal/desktop/pipeline/pipelinedb"
	"github.com/colonyops/hive/pkg/osopen"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// SystemService exposes the desktop app's on-disk locations (data dir, config
// dir, log file, database) to the System settings screen, along with the
// actions that screen offers: open/reveal a path in the OS, pick a new
// directory, and persist point-only data/config directory overrides.
//
// Directory overrides are point-only and take effect after a restart: they are
// written to the bootstrap pointer file (see internal/desktop.Bootstrap) and
// seeded into the environment at next launch. Nothing is moved.
type SystemService struct{}

// NewSystemService constructs the service. It holds no state; every method
// reads live from the desktop path resolvers and the bootstrap file.
func NewSystemService() *SystemService { return &SystemService{} }

// PathInfo describes a single on-disk location surfaced in settings.
type PathInfo struct {
	Path string `json:"path"`
	// Exists reports whether the path is present on disk right now (a log file
	// or database may not exist until first written).
	Exists bool `json:"exists"`
	// Overridden reports whether a stored override backs this location. Only
	// meaningful for the data and config directories; always false otherwise.
	Overridden bool `json:"overridden"`
}

// SystemInfo is the full set of locations shown on the System settings screen.
type SystemInfo struct {
	DataDir   PathInfo `json:"dataDir"`
	ConfigDir PathInfo `json:"configDir"`
	LogFile   PathInfo `json:"logFile"`
	Database  PathInfo `json:"database"`
}

// Info returns the effective locations for this running process plus whether
// the data/config directories are backed by a stored override.
func (s *SystemService) Info() SystemInfo {
	b, _ := desktop.LoadBootstrap()
	return SystemInfo{
		DataDir:   pathInfo(desktop.DataDir(), b.DataDir != ""),
		ConfigDir: pathInfo(desktop.ConfigDir(), b.ConfigDir != ""),
		LogFile:   pathInfo(desktop.LogFile(), false),
		Database:  pathInfo(pipelinedb.DatabasePath(desktop.StateDir()), false),
	}
}

func pathInfo(path string, overridden bool) PathInfo {
	_, err := os.Stat(path)
	return PathInfo{Path: path, Exists: err == nil, Overridden: overridden}
}

// BuildInfo describes the running desktop build so users can see and report the
// exact version they are on from the System settings screen.
type BuildInfo struct {
	Version string `json:"version"`
	// Commit is the short (7-character) git revision the build was cut from.
	Commit string `json:"commit"`
	Date   string `json:"date"`
	// ReleaseURL links to the GitHub release for this build's tag. It is empty
	// for dev/unreleased builds that have no matching published release, so the
	// frontend can hide the link rather than send users to a 404.
	ReleaseURL string `json:"releaseUrl"`
}

// Build returns the version, commit, and date this desktop app was built from,
// plus a link to the matching GitHub release when the build corresponds to a
// published version.
func (s *SystemService) Build() BuildInfo {
	v, c, d := resolvedBuildInfo()
	return BuildInfo{
		Version:    v,
		Commit:     shortCommit(c),
		Date:       d,
		ReleaseURL: releaseURL(v),
	}
}

// OpenPath opens one of the known system locations in the OS default
// application. The path is validated against the current location set so this
// RPC cannot be used to open arbitrary files.
func (s *SystemService) OpenPath(path string) error {
	if err := s.checkAllowed(path); err != nil {
		return err
	}
	return osopen.Open(path)
}

// RevealPath reveals one of the known system locations in the OS file manager.
// Validated the same way as OpenPath.
func (s *SystemService) RevealPath(path string) error {
	if err := s.checkAllowed(path); err != nil {
		return err
	}
	return osopen.Reveal(path)
}

// ChooseDirectory opens a native directory picker and returns the chosen path,
// or "" if the user cancels. Used by the Change… actions before SetDataDir /
// SetConfigDir.
func (s *SystemService) ChooseDirectory(title string) (string, error) {
	app := application.Get()
	if app == nil {
		return "", errors.New("no application context for directory picker")
	}
	dialog := app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true)
	if title != "" {
		dialog.SetTitle(title)
	}
	return dialog.PromptForSingleSelection()
}

// SetDataDir persists a data-directory override. It validates the target and
// creates it if missing, but does not move existing data — the override takes
// effect on the next launch.
func (s *SystemService) SetDataDir(path string) error {
	if err := validateDirOverride(path); err != nil {
		return err
	}
	b, err := desktop.LoadBootstrap()
	if err != nil {
		return err
	}
	b.DataDir = filepath.Clean(path)
	return desktop.SaveBootstrap(b)
}

// SetConfigDir persists a config-directory override (profiles/flows/actions).
// Same semantics as SetDataDir.
func (s *SystemService) SetConfigDir(path string) error {
	if err := validateDirOverride(path); err != nil {
		return err
	}
	b, err := desktop.LoadBootstrap()
	if err != nil {
		return err
	}
	b.ConfigDir = filepath.Clean(path)
	return desktop.SaveBootstrap(b)
}

// ClearDataDir removes the data-directory override, reverting to the default
// location on the next launch.
func (s *SystemService) ClearDataDir() error {
	return clearOverride(func(b *desktop.Bootstrap) { b.DataDir = "" })
}

// ClearConfigDir removes the config-directory override.
func (s *SystemService) ClearConfigDir() error {
	return clearOverride(func(b *desktop.Bootstrap) { b.ConfigDir = "" })
}

// Quit terminates the app so the user can relaunch and apply a directory
// override in one click from the restart-required banner.
func (s *SystemService) Quit() {
	if app := application.Get(); app != nil {
		app.Quit()
	}
}

func clearOverride(mutate func(*desktop.Bootstrap)) error {
	b, err := desktop.LoadBootstrap()
	if err != nil {
		return err
	}
	mutate(&b)
	return desktop.SaveBootstrap(b)
}

// checkAllowed rejects any path that is not one of the four known system
// locations, cleaned for comparison.
func (s *SystemService) checkAllowed(path string) error {
	allowed := map[string]struct{}{
		filepath.Clean(desktop.DataDir()):                           {},
		filepath.Clean(desktop.ConfigDir()):                         {},
		filepath.Clean(desktop.LogFile()):                           {},
		filepath.Clean(pipelinedb.DatabasePath(desktop.StateDir())): {},
	}
	if _, ok := allowed[filepath.Clean(path)]; !ok {
		return fmt.Errorf("path is not a known system location: %s", path)
	}
	return nil
}

// validateDirOverride ensures path is an absolute, creatable, writable
// directory before it is stored as an override.
func validateDirOverride(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("directory path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("directory path must be absolute: %s", path)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}
	probe := filepath.Join(path, ".hive-write-test")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	_ = os.Remove(probe)
	return nil
}
