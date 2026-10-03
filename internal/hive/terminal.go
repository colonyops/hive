package hive

import (
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/domain/terminal"
	"github.com/colonyops/hive/internal/domain/terminal/status"
	"github.com/colonyops/hive/internal/platform/tmux/status"
)

// NewTerminalManager builds the terminal integration manager from config.
// tmux is always enabled; availability is checked lazily by the manager.
func NewTerminalManager(cfg *config.Config, source terminal.PaneSource) *terminal.Manager {
	mgr := terminal.NewManager([]string{"tmux"})
	mgr.Register(newTmuxIntegration(cfg, source))
	return mgr
}

func newTmuxIntegration(cfg *config.Config, source terminal.PaneSource) *tmuxstatus.Integration {
	options := []tmuxstatus.Option{tmuxstatus.WithPaneSource(source)}
	if cfg == nil {
		return tmuxstatus.NewFromPreviewMatchers(nil, options...)
	}

	options = append(options,
		tmuxstatus.WithStatusOptions(StatusOptionsFromConfig(cfg.Terminal.Status, cfg.Tmux.PollInterval)),
		tmuxstatus.WithMissingTolerance(cfg.Terminal.Status.Confirm.Missing.Polls),
	)
	if cfg.Tmux.CaptureRecording.Enabled {
		recorder, err := tmuxstatus.NewJSONCaptureRecorder(cfg.TmuxCaptureRecordingsDir())
		if err != nil {
			log.Warn().Err(err).Msg("failed to enable tmux pane capture recording")
		} else {
			options = append(options, tmuxstatus.WithCaptureRecorder(recorder))
			log.Info().Str("path", recorder.Dir()).Msg("tmux pane capture recording enabled; terminal contents are stored locally")
		}
	}
	return tmuxstatus.NewFromPreviewMatchers(cfg.Tmux.PreviewWindowMatcher, options...)
}

// StatusOptionsFromConfig maps the terminal.status config section onto the
// status tracker's debounce options.
//
// Confirm.Missing is not part of status.Options: the tmux transport reads it
// as a retry count for failed list-panes calls, not as a tracker rule.
func StatusOptionsFromConfig(cfg config.TerminalStatusConfig, pollInterval time.Duration) status.Options {
	opts := status.DefaultOptions()
	opts.PollInterval = pollInterval
	opts.ConfirmIdle = confirmPolicyFromConfig(cfg.Confirm.Idle)
	opts.ConfirmApproval = confirmPolicyFromConfig(cfg.Confirm.Approval)
	return opts
}

func confirmPolicyFromConfig(p config.ConfirmPolicyConfig) status.ConfirmPolicy {
	return status.ConfirmPolicy{
		Polls:         p.Polls,
		MinDuration:   p.MinDuration,
		StableContent: p.StableContent != nil && *p.StableContent,
	}
}
