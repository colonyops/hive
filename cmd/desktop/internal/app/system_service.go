package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/pkg/osopen"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/queries"
	"github.com/colonyops/hive/cmd/desktop/internal/app/hiveconf"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// HiveConfigLocation is the Hive config path this process loaded and whether
// HIVE_CONFIG selected it.
type HiveConfigLocation struct {
	Path                string
	EnvironmentOverride bool
}

type systemOptions struct {
	Paths settings.Paths
	// HiveConfig reads the location rather than holding it: the Hive config is
	// reloadable, and a first run that creates the file changes where it
	// resolves while this service is on screen.
	HiveConfig func() HiveConfigLocation
	OpenPath   func(string) error
	RevealPath func(string) error
}

// SystemService owns the app's on-disk locations and the operations the
// settings screens offer over them: open, reveal, config creation, and the
// point-only data/config directory overrides.
//
// Directory overrides take effect after a restart: they are written to the
// bootstrap pointer file and seeded into the environment at next launch.
// Nothing is moved.
//
// The native directory picker and Quit stay in the adapter — both are GUI,
// not domain.
type SystemService struct {
	paths      settings.Paths
	hiveConfig func() HiveConfigLocation
	openPath   func(string) error
	revealPath func(string) error
}

func newSystemService(opts systemOptions) *SystemService {
	if opts.Paths.SettingsPath == "" {
		b, _ := settings.LoadBootstrap()
		opts.Paths = settings.ResolvePaths(b, settings.ResolveOptions{MockMode: settings.MockMode()})
	}
	if opts.OpenPath == nil {
		opts.OpenPath = osopen.Open
	}
	if opts.RevealPath == nil {
		opts.RevealPath = osopen.Reveal
	}
	if opts.HiveConfig == nil {
		opts.HiveConfig = func() HiveConfigLocation { return HiveConfigLocation{} }
	}
	return &SystemService{
		paths:      opts.Paths,
		hiveConfig: opts.HiveConfig,
		openPath:   opts.OpenPath,
		revealPath: opts.RevealPath,
	}
}

// PathInfo describes a single on-disk location.
type PathInfo struct {
	Path string
	// Exists reports whether the path is present right now (a log file or
	// database may not exist until first written).
	Exists bool
	// Overridden reports whether an explicit override selected this location.
	Overridden bool
}

// SystemInfo is the full set of locations the settings screens show.
type SystemInfo struct {
	DataDir         PathInfo
	ConfigDir       PathInfo
	LogFile         PathInfo
	Database        PathInfo
	AgentWorkspaces PathInfo
	HiveConfig      PathInfo
}

// Info returns the effective locations for this process and their override
// state.
func (s *SystemService) Info(context.Context) SystemInfo {
	hiveConfig := s.hiveConfig()
	return SystemInfo{
		DataDir:         pathInfo(s.paths.DataDir, s.paths.DataDirOverridden),
		ConfigDir:       pathInfo(s.paths.ConfigDir, s.paths.ConfigDirOverridden),
		LogFile:         pathInfo(s.paths.LogFile, false),
		Database:        pathInfo(queries.DatabasePath(s.paths.StateDir), false),
		AgentWorkspaces: pathInfo(s.paths.AgentWorkspacesDir, false),
		HiveConfig:      pathInfo(hiveConfig.Path, hiveConfig.EnvironmentOverride),
	}
}

func pathInfo(path string, overridden bool) PathInfo {
	_, err := os.Stat(path)
	return PathInfo{Path: path, Exists: err == nil, Overridden: overridden}
}

// OpenPath opens one of the known system locations in the OS default
// application.
func (s *SystemService) OpenPath(_ context.Context, path string) error {
	if err := s.checkAllowed(path); err != nil {
		return err
	}
	return Wrap(s.openPath(path), KindInternal, "opening %s", path)
}

// RevealPath reveals one of the known system locations in the OS file
// manager.
func (s *SystemService) RevealPath(_ context.Context, path string) error {
	if err := s.checkAllowed(path); err != nil {
		return err
	}
	return Wrap(s.revealPath(path), KindInternal, "revealing %s", path)
}

// OpenHiveConfig creates the resolved Hive config when needed, then opens it
// in the OS default application.
func (s *SystemService) OpenHiveConfig(_ context.Context) error {
	path := s.hiveConfig().Path
	if path == "" {
		return Errorf(KindInternal, "Hive config path is unavailable")
	}
	if err := hiveconf.Create(path); err != nil {
		return Wrap(err, KindInternal, "creating the Hive config")
	}
	return Wrap(s.openPath(path), KindInternal, "opening %s", path)
}

// SetDataDir persists a data-directory override. It validates the target and
// creates it if missing, but does not move existing data.
func (s *SystemService) SetDataDir(_ context.Context, path string) error {
	if err := validateDirOverride(path); err != nil {
		return err
	}
	b, err := settings.LoadBootstrap()
	if err != nil {
		return Wrap(err, KindInternal, "reading the bootstrap file")
	}
	b.DataDir = filepath.Clean(path)
	return Wrap(settings.SaveBootstrap(b), KindInternal, "saving the bootstrap file")
}

// SetConfigDir persists a config-directory override (flows, actions). Same
// semantics as SetDataDir.
func (s *SystemService) SetConfigDir(_ context.Context, path string) error {
	if err := validateDirOverride(path); err != nil {
		return err
	}
	b, err := settings.LoadBootstrap()
	if err != nil {
		return Wrap(err, KindInternal, "reading the bootstrap file")
	}
	b.ConfigDir = filepath.Clean(path)
	return Wrap(settings.SaveBootstrap(b), KindInternal, "saving the bootstrap file")
}

// ClearDataDir removes the data-directory override, reverting to the default
// location on the next launch.
func (s *SystemService) ClearDataDir(context.Context) error {
	return clearOverride(func(b *settings.Bootstrap) { b.DataDir = "" })
}

// ClearConfigDir removes the config-directory override.
func (s *SystemService) ClearConfigDir(context.Context) error {
	return clearOverride(func(b *settings.Bootstrap) { b.ConfigDir = "" })
}

func clearOverride(mutate func(*settings.Bootstrap)) error {
	b, err := settings.LoadBootstrap()
	if err != nil {
		return Wrap(err, KindInternal, "reading the bootstrap file")
	}
	mutate(&b)
	return Wrap(settings.SaveBootstrap(b), KindInternal, "saving the bootstrap file")
}

// checkAllowed rejects any path that is not one of the known system
// locations, cleaned for comparison. This is a security control, not a
// convenience: without it OpenPath is an arbitrary-file-open RPC.
func (s *SystemService) checkAllowed(path string) error {
	allowed := map[string]struct{}{
		filepath.Clean(s.paths.DataDir):                        {},
		filepath.Clean(s.paths.ConfigDir):                      {},
		filepath.Clean(s.paths.LogFile):                        {},
		filepath.Clean(queries.DatabasePath(s.paths.StateDir)): {},
		filepath.Clean(s.paths.AgentWorkspacesDir):             {},
		filepath.Clean(s.paths.ReportsDir):                     {},
	}
	if hiveConfig := s.hiveConfig(); hiveConfig.Path != "" {
		allowed[filepath.Clean(hiveConfig.Path)] = struct{}{}
	}
	if _, ok := allowed[filepath.Clean(path)]; !ok {
		return Errorf(KindInvalid, "path is not a known system location: %s", path)
	}
	return nil
}

// validateDirOverride ensures path is an absolute, creatable, writable
// directory before it is stored. The write probe is the point: a directory
// that exists but cannot be written to would fail at next launch, with the
// app already pointed at it.
func validateDirOverride(path string) error {
	if strings.TrimSpace(path) == "" {
		return Errorf(KindInvalid, "directory path is required")
	}
	if !filepath.IsAbs(path) {
		return Errorf(KindInvalid, "directory path must be absolute: %s", path)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return Wrap(err, KindInvalid, "create directory")
	}
	info, err := os.Stat(path)
	if err != nil {
		return Wrap(err, KindInvalid, "stat directory")
	}
	if !info.IsDir() {
		return Errorf(KindInvalid, "not a directory: %s", path)
	}
	probe := filepath.Join(path, ".hive-write-test")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		return Wrap(err, KindInvalid, "directory is not writable")
	}
	_ = os.Remove(probe)
	return nil
}
