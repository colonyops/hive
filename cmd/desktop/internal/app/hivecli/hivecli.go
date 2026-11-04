// Package hivecli installs the `hive` command for Hive Desktop: a symlink at
// ~/.local/bin/hive that points at the desktop executable, which runs the hive
// CLI when it is invoked under that name. The link is the only record of
// ownership. The package writes or removes a path only when it is that link,
// and never touches a `hive` the user installed another way.
package hivecli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Owner says who put the file at the install path.
type Owner string

const (
	OwnerNone    Owner = "none"
	OwnerApp     Owner = "app"
	OwnerForeign Owner = "foreign"
)

// Skip says why the app does not manage the command on this run.
type Skip string

const (
	SkipNone Skip = ""
	// SkipDev is a development build. Its executable lives in a worktree, so a
	// link to it would break when the checkout moves.
	SkipDev Skip = "dev"
	// SkipTranslocated is a macOS app running from a read-only randomized
	// mount, which disappears when the app quits.
	SkipTranslocated Skip = "translocated"
	// SkipUnwritable is an install directory that cannot be created or written.
	SkipUnwritable Skip = "unwritable"
	// SkipUnrecognizable is an executable the ownership rule would not
	// recognize as the app, so a link to it could never be removed again.
	SkipUnrecognizable Skip = "unrecognizable"
)

const (
	desktopBinaryName = "hive-desktop"
	commandName       = "hive"
	versionTimeout    = 2 * time.Second
)

// Status is what Settings and first run render.
type Status struct {
	// Skip is non-empty when the app is not managing the command, and Detail
	// carries the underlying error for SkipUnwritable.
	Skip   Skip   `json:"skip"`
	Detail string `json:"detail"`
	// InstallPath is where the command lives, Owner who put the file there,
	// and Target the link's destination (empty for a regular file).
	InstallPath string `json:"installPath"`
	Owner       Owner  `json:"owner"`
	Target      string `json:"target"`
	// Resolved is where `hive` leads on the login shell's PATH, empty when
	// nothing does.
	Resolved string `json:"resolved"`
	// Shadowed is true when Resolved is some other file than InstallPath.
	Shadowed bool `json:"shadowed"`
	// OnPath is true when the install directory is on the login shell's PATH.
	OnPath bool `json:"onPath"`
	// AppVersion is this build's version and CommandVersion what Resolved
	// prints, empty when it printed nothing parseable.
	AppVersion     string `json:"appVersion"`
	CommandVersion string `json:"commandVersion"`
	Differs        bool   `json:"differs"`
}

// Options wires a Manager to its environment. Home, Version, and UserPath are
// required; the rest have working defaults.
type Options struct {
	Home    string
	Version string
	// Executable is the running binary with symlinks resolved. The default is
	// os.Executable followed by filepath.EvalSymlinks.
	Executable func() (string, error)
	// UserPath is the login shell's PATH, without the prefixes the app adds
	// for its own lookups.
	UserPath func(context.Context) string
	// Environ is the environment a probe of `hive --version` runs with.
	Environ func(context.Context) []string
	// Probe returns the output of `<path> --version`.
	Probe  func(ctx context.Context, path string, env []string) (string, error)
	Logger zerolog.Logger
}

// Manager reconciles the install path with the user's choice.
type Manager struct {
	opts Options

	mu sync.Mutex
	// problem is the last reconcile's failure, kept so Status explains an
	// unwritable directory without trying to write again.
	problem       Skip
	problemDetail string
}

func New(opts Options) *Manager {
	if opts.Executable == nil {
		opts.Executable = runningExecutable
	}
	if opts.Environ == nil {
		opts.Environ = func(context.Context) []string { return os.Environ() }
	}
	if opts.Probe == nil {
		opts.Probe = func(ctx context.Context, path string, env []string) (string, error) {
			return RunVersion(ctx, path, env, versionTimeout)
		}
	}
	return &Manager{opts: opts}
}

func runningExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// InstallPath is where the command lives.
func (m *Manager) InstallPath() string {
	return filepath.Join(m.opts.Home, ".local", "bin", commandName)
}

// Reconcile makes the install path match enabled and returns the resulting
// status. A nil enabled means the user was never asked: nothing is written.
func (m *Manager) Reconcile(ctx context.Context, enabled *bool) Status {
	m.setProblem(SkipNone, "")
	if enabled != nil && m.unmanaged() == SkipNone {
		if err := m.apply(*enabled); err != nil {
			m.opts.Logger.Warn().Err(err).Str("path", m.InstallPath()).Msg("hive command: reconcile failed")
			m.setProblem(SkipUnwritable, err.Error())
		}
	}
	return m.Status(ctx)
}

func (m *Manager) setProblem(skip Skip, detail string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.problem, m.problemDetail = skip, detail
}

// Status reads the install path and the login shell's view of `hive`.
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	problem, detail := m.problem, m.problemDetail
	m.mu.Unlock()

	st := Status{
		InstallPath: m.InstallPath(),
		AppVersion:  m.opts.Version,
		Skip:        m.unmanaged(),
	}
	if st.Skip == SkipNone {
		st.Skip, st.Detail = problem, detail
	}
	st.Owner, st.Target = m.inspect()

	var searched []string
	if m.opts.UserPath != nil {
		searched = filepath.SplitList(m.opts.UserPath(ctx))
	}
	installDir := filepath.Dir(st.InstallPath)
	for _, entry := range searched {
		if filepath.Clean(entry) == installDir {
			st.OnPath = true
			break
		}
	}
	st.Resolved = lookPath(searched, commandName)
	st.Shadowed = st.Resolved != "" && filepath.Clean(st.Resolved) != st.InstallPath

	if st.Resolved != "" {
		st.CommandVersion = m.commandVersion(ctx, st.Resolved)
		st.Differs = st.CommandVersion != st.AppVersion
	}
	return st
}

func (m *Manager) commandVersion(ctx context.Context, path string) string {
	out, err := m.opts.Probe(ctx, path, m.opts.Environ(ctx))
	if err != nil {
		m.opts.Logger.Debug().Err(err).Str("path", path).Msg("hive command: version probe failed")
		return ""
	}
	version, _ := ParseVersion(out)
	return version
}

func lookPath(dirs []string, name string) string {
	for _, dir := range dirs {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

// unmanaged reports why this process must not manage the command.
func (m *Manager) unmanaged() Skip {
	if m.opts.Version == "dev" {
		return SkipDev
	}
	exe, err := m.opts.Executable()
	if err != nil {
		return SkipUnrecognizable
	}
	if strings.Contains(exe, "/AppTranslocation/") {
		return SkipTranslocated
	}
	if filepath.Base(exe) != desktopBinaryName {
		return SkipUnrecognizable
	}
	return SkipNone
}

// inspect classifies the file at the install path without following it.
func (m *Manager) inspect() (Owner, string) {
	path := m.InstallPath()
	info, err := os.Lstat(path)
	if err != nil {
		return OwnerNone, ""
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return OwnerForeign, ""
	}
	target, err := os.Readlink(path)
	if err != nil {
		return OwnerForeign, ""
	}
	if isAppTarget(target) {
		return OwnerApp, target
	}
	return OwnerForeign, target
}

// isAppTarget matches the desktop executable in a bundle
// (Hive.app/Contents/MacOS/hive-desktop) or a Linux install. Both end in the
// binary name, so the base name decides.
func isAppTarget(target string) bool {
	return filepath.Base(target) == desktopBinaryName
}

func (m *Manager) apply(enabled bool) error {
	owner, target := m.inspect()
	path := m.InstallPath()
	if !enabled {
		if owner == OwnerApp {
			return removeIfExists(path)
		}
		return nil
	}
	if owner == OwnerForeign {
		return nil
	}
	exe, err := m.opts.Executable()
	if err != nil {
		return fmt.Errorf("resolve the app executable: %w", err)
	}
	if owner == OwnerApp && pointsAt(target, exe) {
		return nil
	}
	return replaceLink(path, exe)
}

func pointsAt(target, exe string) bool {
	if filepath.Clean(target) == filepath.Clean(exe) {
		return true
	}
	resolved, err := filepath.EvalSymlinks(target)
	return err == nil && resolved == exe
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// replaceLink points path at exe. It renames a fresh link over the old one so
// a shell never finds the command missing during a re-point.
func replaceLink(path, exe string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	staged := filepath.Join(dir, fmt.Sprintf(".%s.%d.tmp", commandName, os.Getpid()))
	_ = os.Remove(staged)
	if err := os.Symlink(exe, staged); err != nil {
		return err
	}
	if err := os.Rename(staged, path); err != nil {
		_ = os.Remove(staged)
		return err
	}
	return nil
}
