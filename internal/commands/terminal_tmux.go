package commands

import (
	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/config"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
)

func newTmuxIntegration(cfg *config.Config) *terminaltmux.Integration {
	if cfg == nil {
		return terminaltmux.NewFromPreviewMatchers(nil)
	}

	var options []terminaltmux.Option
	if cfg.Tmux.CaptureRecording.Enabled {
		recorder, err := terminaltmux.NewJSONLCaptureRecorder(cfg.TmuxCaptureRecordingsDir())
		if err != nil {
			log.Warn().Err(err).Msg("failed to enable tmux pane capture recording")
		} else {
			options = append(options, terminaltmux.WithCaptureRecorder(recorder))
			log.Info().Str("path", recorder.Path()).Msg("tmux pane capture recording enabled; terminal contents are stored locally")
		}
	}
	return terminaltmux.NewFromPreviewMatchers(cfg.Tmux.PreviewWindowMatcher, options...)
}
