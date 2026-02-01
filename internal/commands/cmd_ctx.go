package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hay-kot/hive/internal/core/git"
	"github.com/hay-kot/hive/internal/printer"
	"github.com/rs/zerolog"
	"github.com/urfave/cli/v3"
)

type CtxCmd struct {
	flags *Flags

	// Shared flags
	repo   string
	shared bool

	// prune flag
	olderThan string
}

// NewCtxCmd creates a new ctx command.
func NewCtxCmd(flags *Flags) *CtxCmd {
	return &CtxCmd{flags: flags}
}

// Register adds the ctx command to the application.
func (cmd *CtxCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "ctx",
		Usage: "Manage context directories for inter-agent communication",
		Description: `Context commands manage shared directories for agents.

Each repository gets its own context directory at $XDG_DATA_HOME/hive/context/{owner}/{repo}/.
Use 'hive ctx init' in a git repository to create a .hive symlink pointing to this directory.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "repo",
				Aliases:     []string{"r"},
				Usage:       "target a specific repository context (owner/repo)",
				Destination: &cmd.repo,
			},
			&cli.BoolFlag{
				Name:        "shared",
				Aliases:     []string{"s"},
				Usage:       "use the shared context directory",
				Destination: &cmd.shared,
			},
		},
		Commands: []*cli.Command{
			cmd.initCmd(),
			cmd.pruneCmd(),
		},
	})

	return app
}

func (cmd *CtxCmd) initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create symlink to context directory",
		Description: `Creates a symlink in the current directory pointing to the context directory.

The symlink name is configured via context.symlink_name (default: .hive).
The target is $XDG_DATA_HOME/hive/context/{owner}/{repo}/.`,
		Action: cmd.runInit,
	}
}

func (cmd *CtxCmd) pruneCmd() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "Delete old files from context directory",
		Description: `Deletes files older than the specified duration.

Example: hive ctx prune --older-than 7d`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "older-than",
				Usage:       "delete files older than this duration (e.g., 7d, 24h)",
				Destination: &cmd.olderThan,
				Required:    true,
			},
		},
		Action: cmd.runPrune,
	}
}

func (cmd *CtxCmd) runInit(ctx context.Context, c *cli.Command) error {
	log := zerolog.Ctx(ctx)
	p := printer.Ctx(ctx)

	log.Info().Str("repo", cmd.repo).Bool("shared", cmd.shared).Msg("ctx init invoked")

	ctxDir, err := cmd.resolveContextDir(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to resolve context directory")
		return err
	}

	log.Debug().Str("ctx_dir", ctxDir).Msg("resolved context directory")

	// Create the context directory if it doesn't exist
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		log.Error().Err(err).Str("ctx_dir", ctxDir).Msg("failed to create context directory")
		return fmt.Errorf("create context directory: %w", err)
	}

	symlinkName := cmd.flags.Config.Context.SymlinkName
	symlinkPath := filepath.Join(".", symlinkName)

	// Check if symlink already exists
	if info, err := os.Lstat(symlinkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(symlinkPath)
			if target == ctxDir {
				log.Debug().Str("symlink", symlinkName).Str("target", ctxDir).Msg("symlink already exists")
				p.Infof("Symlink already exists: %s -> %s", symlinkName, ctxDir)
				return nil
			}
			log.Warn().Str("symlink", symlinkName).Str("current_target", target).Str("expected_target", ctxDir).Msg("symlink points to wrong target")
			return fmt.Errorf("symlink %s exists but points to %s, not %s", symlinkName, target, ctxDir)
		}
		log.Error().Str("path", symlinkName).Msg("path exists but is not a symlink")
		return fmt.Errorf("%s already exists and is not a symlink", symlinkName)
	}

	if err := os.Symlink(ctxDir, symlinkPath); err != nil {
		log.Error().Err(err).Str("symlink", symlinkName).Str("target", ctxDir).Msg("failed to create symlink")
		return fmt.Errorf("create symlink: %w", err)
	}

	log.Info().Str("symlink", symlinkName).Str("target", ctxDir).Msg("symlink created successfully")
	p.Successf("Created symlink: %s -> %s", symlinkName, ctxDir)
	return nil
}

func (cmd *CtxCmd) runPrune(ctx context.Context, c *cli.Command) error {
	log := zerolog.Ctx(ctx)
	p := printer.Ctx(ctx)

	log.Info().Str("older_than", cmd.olderThan).Msg("ctx prune invoked")

	ctxDir, err := cmd.resolveContextDir(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to resolve context directory")
		return err
	}

	duration, err := parseDuration(cmd.olderThan)
	if err != nil {
		log.Warn().Err(err).Str("duration", cmd.olderThan).Msg("invalid duration format")
		return fmt.Errorf("invalid duration: %w", err)
	}

	cutoff := time.Now().Add(-duration)
	count := 0

	log.Debug().Str("ctx_dir", ctxDir).Time("cutoff", cutoff).Msg("pruning files")

	entries, err := os.ReadDir(ctxDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug().Str("ctx_dir", ctxDir).Msg("context directory does not exist")
			p.Infof("Context directory does not exist")
			return nil
		}
		log.Error().Err(err).Str("ctx_dir", ctxDir).Msg("failed to read directory")
		return fmt.Errorf("read directory: %w", err)
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			log.Warn().Err(err).Str("entry", entry.Name()).Msg("failed to get file info")
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(ctxDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				log.Warn().Err(err).Str("path", path).Msg("failed to remove file")
				p.Warnf("Failed to remove %s: %v", entry.Name(), err)
				continue
			}
			log.Debug().Str("path", path).Msg("removed old file")
			count++
		}
	}

	log.Info().Int("count", count).Str("older_than", cmd.olderThan).Msg("prune completed")
	p.Successf("Removed %d file(s) older than %s", count, cmd.olderThan)
	return nil
}

func (cmd *CtxCmd) resolveContextDir(ctx context.Context) (string, error) {
	if cmd.shared {
		return cmd.flags.Config.SharedContextDir(), nil
	}

	if cmd.repo != "" {
		parts := strings.SplitN(cmd.repo, "/", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid repo format, expected owner/repo: %s", cmd.repo)
		}
		return cmd.flags.Config.RepoContextDir(parts[0], parts[1]), nil
	}

	// Detect from current directory
	remote, err := cmd.flags.Service.DetectRemote(ctx, ".")
	if err != nil {
		return "", fmt.Errorf("detect remote (are you in a git repository?): %w", err)
	}

	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return "", fmt.Errorf("could not extract owner/repo from remote: %s", remote)
	}

	return cmd.flags.Config.RepoContextDir(owner, repo), nil
}

func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix (not supported by time.ParseDuration)
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid days: %s", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
