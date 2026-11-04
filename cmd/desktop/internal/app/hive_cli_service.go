package app

import (
	"context"
	"os"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/hivecli"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/platform/execenv"
)

// HiveCLIStatus is the `hive` command as Settings and first run show it.
type HiveCLIStatus struct {
	// Install is hive_cli.install_command, nil until the user was asked.
	Install *bool          `json:"install"`
	Command hivecli.Status `json:"command"`
}

// HiveCLIService installs the `hive` command: a link at ~/.local/bin/hive to
// this executable, which runs the hive CLI when started under that name.
type HiveCLIService struct {
	store   *settings.Store
	manager *hivecli.Manager
	logger  zerolog.Logger
}

func newHiveCLIService(store *settings.Store, execEnv *execenv.Resolver, version string, logger zerolog.Logger) (*HiveCLIService, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, Wrap(err, KindInternal, "resolving the home directory")
	}
	return &HiveCLIService{
		store: store,
		manager: hivecli.New(hivecli.Options{
			Home:     home,
			Version:  version,
			UserPath: execEnv.UserPath,
			Environ:  execEnv.Environ,
			Logger:   logger,
		}),
		logger: logger,
	}, nil
}

func (s *HiveCLIService) install() (*bool, error) {
	cfg, err := s.store.Effective()
	if err != nil {
		return nil, Wrap(err, KindInternal, "reading settings")
	}
	return cfg.HiveCLI.InstallCommand, nil
}

// Status reports the command without changing anything.
func (s *HiveCLIService) Status(ctx context.Context) (HiveCLIStatus, error) {
	install, err := s.install()
	if err != nil {
		return HiveCLIStatus{}, err
	}
	return HiveCLIStatus{Install: install, Command: s.manager.Status(ctx)}, nil
}

// Reconcile applies the saved choice. Startup runs it, so an app update or a
// moved app re-points the link. A user who was never asked is left alone.
func (s *HiveCLIService) Reconcile(ctx context.Context) {
	install, err := s.install()
	if err != nil {
		s.logger.Warn().Err(err).Msg("hive command: settings unreadable; leaving the command as it is")
		return
	}
	s.manager.Reconcile(ctx, install)
}

// SetInstall saves the choice and applies it.
func (s *HiveCLIService) SetInstall(ctx context.Context, enabled bool) (HiveCLIStatus, error) {
	if _, err := s.store.Update(func(current *settings.Settings) error {
		current.HiveCLI.InstallCommand = &enabled
		return nil
	}); err != nil {
		return HiveCLIStatus{}, Wrap(err, KindInternal, "saving settings")
	}
	return HiveCLIStatus{Install: &enabled, Command: s.manager.Reconcile(ctx, &enabled)}, nil
}
