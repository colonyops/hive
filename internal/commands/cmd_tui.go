package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/tmux"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/tui"
	"github.com/colonyops/hive/pkg/profiler"
)

type TuiCmd struct {
	flags *Flags
	app   *hive.App
}

// NewTuiCmd creates a new tui command
func NewTuiCmd(flags *Flags, app *hive.App) *TuiCmd {
	return &TuiCmd{
		flags: flags,
		app:   app,
	}
}

// Flags returns the TUI-specific flags for registration on the root command
func (cmd *TuiCmd) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.IntFlag{
			Name:        "profiler-port",
			Usage:       "enable pprof HTTP endpoint on specified port (e.g., 6060)",
			Sources:     cli.EnvVars("HIVE_PROFILER_PORT"),
			Destination: &cmd.flags.ProfilerPort,
		},
	}
}

// Run executes the TUI. Exported for use as default command.
func (cmd *TuiCmd) Run(ctx context.Context, c *cli.Command) error {
	return cmd.run(ctx, c)
}

func (cmd *TuiCmd) run(ctx context.Context, _ *cli.Command) error {
	var warnings []string
	if os.Getenv("TMUX") == "" {
		warnings = append(warnings, "Not running inside tmux. Some features (preview, spawn) require tmux.")
	}

	// Start profiler server if enabled
	if cmd.flags.ProfilerPort > 0 {
		profServer := profiler.New(cmd.flags.ProfilerPort)
		if err := profServer.Start(ctx); err != nil {
			return fmt.Errorf("failed to start profiler: %w", err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := profServer.Shutdown(shutdownCtx); err != nil {
				log.Error().Err(err).Msg("failed to shutdown profiler server")
			}
		}()
		log.Info().
			Str("url", fmt.Sprintf("http://%s/debug/pprof/", profServer.Addr())).
			Msg("profiler endpoint available")
	}

	// Detect current repository remote for highlighting current repo
	localRemote, _ := cmd.app.Sessions.DetectRemote(ctx, ".")

	// Create terminal integration manager (tmux always enabled)
	termMgr := terminal.NewManager([]string{"tmux"})
	// Register tmux integration with preview window matcher patterns
	preferredWindows := cmd.app.Config.Tmux.PreviewWindowMatcher
	tmuxIntegration := tmux.New(preferredWindows)
	if tmuxIntegration.Available() {
		termMgr.Register(tmuxIntegration)
	}

	for {
		deps := tui.Deps{
			Config:          cmd.app.Config,
			Service:         cmd.app.Sessions,
			MsgStore:        cmd.app.Messages,
			Bus:             cmd.app.Bus,
			TerminalManager: termMgr,
			PluginManager:   cmd.app.Plugins,
			DB:              cmd.app.DB,
			KVStore:         cmd.app.KV,
			Renderer:        cmd.app.Renderer,
			BuildInfo: tui.BuildInfo{
				Version: cmd.app.Build.Version,
				Commit:  cmd.app.Build.Commit,
				Date:    cmd.app.Build.Date,
			},
			DoctorService: cmd.app.Doctor,
		}
		opts := tui.Opts{
			LocalRemote: localRemote,
			Warnings:    warnings,
			ConfigPath:  cmd.flags.ConfigPath,
		}

		restoreOutput := cmd.app.Sessions.SilenceOutput()

		m := tui.New(deps, opts)
		p := tea.NewProgram(m)

		finalModel, err := p.Run()
		restoreOutput()
		if err != nil {
			return fmt.Errorf("run tui: %w", err)
		}

		model := finalModel.(tui.Model)

		// Handle pending session creation
		if pending := model.PendingCreate(); pending != nil {
			source, _ := os.Getwd()
			_, err := cmd.app.Sessions.CreateSession(ctx, hive.CreateOptions{
				Name:   pending.Name,
				Remote: pending.Remote,
				Source: source,
			})
			if err != nil {
				fmt.Printf("Error creating session: %v\n", err)
				fmt.Println("Press Enter to continue...")
				_, _ = fmt.Scanln()
			}
			continue // Restart TUI
		}

		break // Normal exit
	}

	return nil
}
