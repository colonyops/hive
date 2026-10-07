package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hivecli"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/pkg/logutils"
)

const hiveVersionTimeout = 2 * time.Second

// HiveCLIStatus is the `hive` command as first run and Settings ▸ Hive CLI
// show it: whether the app installs it, what is at the install path, and what
// the user's shell would actually run.
type HiveCLIStatus struct {
	// Asked is false until first run or Settings records a choice.
	Asked   bool `json:"asked"`
	Enabled bool `json:"enabled"`
	// Unsupported says why this build cannot install the command, or "".
	Unsupported string       `json:"unsupported"`
	Link        hivecli.Link `json:"link"`
	// Conflict means the command is wanted but another program's file holds
	// the install path.
	Conflict      bool   `json:"conflict"`
	LinkDir       string `json:"linkDir"`
	LinkDirOnPath bool   `json:"linkDirOnPath"`
	// Resolved is the `hive` the login shell runs, or "" when there is none.
	Resolved string `json:"resolved"`
	// Shadowed means the app's command is installed but an earlier PATH entry
	// wins.
	Shadowed       bool   `json:"shadowed"`
	AppVersion     string `json:"appVersion"`
	CommandVersion string `json:"commandVersion"`
	VersionsDiffer bool   `json:"versionsDiffer"`
}

type hiveCLIOptions struct {
	Store      *settings.Store
	Home       func() (string, error)
	Executable func() (string, error)
	Version    string
	// ShellPath is the login shell's PATH (execenv.Resolver.ShellPath).
	ShellPath func(context.Context) string
	// CommandVersion runs `<path> --version`. nil runs it for real.
	CommandVersion func(ctx context.Context, path string) (string, error)
	Logger         zerolog.Logger
}

// HiveCLIService installs the `hive` command as a link to this executable and
// keeps it pointing here.
type HiveCLIService struct {
	opts hiveCLIOptions
}

func newHiveCLIService(opts hiveCLIOptions) *HiveCLIService {
	if opts.CommandVersion == nil {
		opts.CommandVersion = runCommandVersion
	}
	opts.Logger = logutils.Component(opts.Logger, "hivecli")
	return &HiveCLIService{opts: opts}
}

// Status reports the command without changing anything.
func (s *HiveCLIService) Status(ctx context.Context) (HiveCLIStatus, error) {
	cfg, err := s.opts.Store.Effective()
	if err != nil {
		return HiveCLIStatus{}, Wrap(err, KindInternal, "reading settings")
	}
	home, err := s.opts.Home()
	if err != nil {
		return HiveCLIStatus{}, Wrap(err, KindInternal, "resolving the home directory")
	}
	exe, err := s.executable()
	if err != nil {
		return HiveCLIStatus{}, Wrap(err, KindInternal, "resolving the Hive executable")
	}
	linkPath := hivecli.LinkPath(home)
	link, err := hivecli.Inspect(linkPath)
	if err != nil {
		return HiveCLIStatus{}, Wrap(err, KindInternal, "reading %s", linkPath)
	}

	choice := cfg.HiveCLI.InstallCommand
	status := HiveCLIStatus{
		Asked:       choice != nil,
		Enabled:     choice != nil && *choice,
		Unsupported: hivecli.Unsupported(exe, s.opts.Version),
		Link:        link,
		LinkDir:     filepath.Dir(linkPath),
		AppVersion:  s.opts.Version,
	}
	status.Conflict = status.Enabled && link.Exists && !link.AppOwned

	shellPath := s.opts.ShellPath(ctx)
	status.LinkDirOnPath = hivecli.OnPath(shellPath, status.LinkDir)
	status.Resolved = hivecli.Resolve(shellPath)
	status.Shadowed = link.AppOwned && status.Resolved != linkPath
	if status.Resolved != "" {
		out, err := s.opts.CommandVersion(ctx, status.Resolved)
		if err != nil {
			s.opts.Logger.Debug().Err(err).Str("path", status.Resolved).Msg("hive --version failed")
		}
		status.CommandVersion = hivecli.ParseVersion(out)
		status.VersionsDiffer = status.CommandVersion != "" && !hivecli.SameVersion(status.CommandVersion, status.AppVersion)
	}
	return status, nil
}

// SetInstall records the choice and applies it.
func (s *HiveCLIService) SetInstall(ctx context.Context, install bool) (HiveCLIStatus, error) {
	_, err := s.opts.Store.Update(func(current *settings.Settings) error {
		current.HiveCLI.InstallCommand = &install
		return nil
	})
	if err != nil {
		return HiveCLIStatus{}, Wrap(err, KindInternal, "saving settings")
	}
	if err := s.Sync(ctx); err != nil {
		return HiveCLIStatus{}, err
	}
	return s.Status(ctx)
}

// Sync makes the install path match the recorded choice. An install that was
// never asked is left alone, and so is everything when this build cannot back
// the command: a dev build has its own settings, and its "off" must not remove
// the link the installed app made.
func (s *HiveCLIService) Sync(context.Context) error {
	cfg, err := s.opts.Store.Effective()
	if err != nil {
		return Wrap(err, KindInternal, "reading settings")
	}
	choice := cfg.HiveCLI.InstallCommand
	if choice == nil {
		return nil
	}
	home, err := s.opts.Home()
	if err != nil {
		return Wrap(err, KindInternal, "resolving the home directory")
	}
	exe, err := s.executable()
	if err != nil {
		return Wrap(err, KindInternal, "resolving the Hive executable")
	}
	if hivecli.Unsupported(exe, s.opts.Version) != "" {
		return nil
	}
	linkPath := hivecli.LinkPath(home)
	action, err := hivecli.Sync(linkPath, exe, *choice)
	if err != nil {
		return Wrap(err, KindInternal, "installing the hive command at %s", linkPath)
	}
	if action != hivecli.ActionNone {
		s.opts.Logger.Info().Str("action", string(action)).Str("path", linkPath).Str("target", exe).Msg("hive command")
	}
	return nil
}

// executable resolves symlinks so the link targets the real binary, not
// another link that could move.
func (s *HiveCLIService) executable() (string, error) {
	exe, err := s.opts.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func runCommandVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, hiveVersionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	return string(out), err
}
