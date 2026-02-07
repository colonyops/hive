package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"

	"github.com/hay-kot/hive/internal/hive"
	"github.com/hay-kot/hive/internal/integration/terminal"
	"github.com/hay-kot/hive/internal/integration/terminal/tmux"
	"github.com/hay-kot/hive/internal/profiler"
	"github.com/hay-kot/hive/internal/tui"
)

type TuiCmd struct {
	flags *Flags
}

// NewTuiCmd creates a new tui command
func NewTuiCmd(flags *Flags) *TuiCmd {
	return &TuiCmd{
		flags: flags,
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
	localRemote, _ := cmd.flags.Service.DetectRemote(ctx, ".")

	// Use SQLite message store
	msgStore := cmd.flags.MsgStore

	// Create terminal integration manager (tmux always enabled)
	termMgr := terminal.NewManager([]string{"tmux"})
	// Register tmux integration with preview window matcher patterns
	preferredWindows := cmd.flags.Config.Tmux.PreviewWindowMatcher
	tmuxIntegration := tmux.New(preferredWindows)
	if tmuxIntegration.Available() {
		termMgr.Register(tmuxIntegration)
	}

	for {
		opts := tui.Options{
			LocalRemote:     localRemote,
			MsgStore:        msgStore,
			TerminalManager: termMgr,
			PluginManager:   cmd.flags.PluginManager,
			DB:              cmd.flags.DB,
		}

		m := tui.New(cmd.flags.Service, cmd.flags.Config, opts)
		p := tea.NewProgram(m)

		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("run tui: %w", err)
		}

		model := finalModel.(tui.Model)

		// Handle pending session creation
		if pending := model.PendingCreate(); pending != nil {
			source, _ := os.Getwd()
			_, err := cmd.flags.Service.CreateSession(ctx, hive.CreateOptions{
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
