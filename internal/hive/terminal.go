package hive

import (
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/status"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
)

// NewTerminalManager builds the terminal integration manager from config.
// tmux is always enabled; availability is checked lazily by the manager.
func NewTerminalManager(cfg *config.Config) *terminal.Manager {
	mgr := terminal.NewManager([]string{"tmux"})
	mgr.Register(newTmuxIntegration(cfg))
	return mgr
}

func newTmuxIntegration(cfg *config.Config) *terminaltmux.Integration {
	if cfg == nil {
		return terminaltmux.NewFromPreviewMatchers(nil)
	}

	options := []terminaltmux.Option{
		terminaltmux.WithStatusOptions(status.OptionsFromConfig(cfg.Terminal.Status, cfg.Tmux.PollInterval)),
		terminaltmux.WithMissingTolerance(cfg.Terminal.Status.Confirm.Missing.Polls),
	}
	if cfg.Tmux.CaptureRecording.Enabled {
		recorder, err := terminaltmux.NewJSONCaptureRecorder(cfg.TmuxCaptureRecordingsDir())
		if err != nil {
			log.Warn().Err(err).Msg("failed to enable tmux pane capture recording")
		} else {
			options = append(options, terminaltmux.WithCaptureRecorder(recorder))
			log.Info().Str("path", recorder.Dir()).Msg("tmux pane capture recording enabled; terminal contents are stored locally")
		}
	}
	return terminaltmux.NewFromPreviewMatchers(cfg.Tmux.PreviewWindowMatcher, options...)
}
