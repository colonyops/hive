// Package commands hosts CLI command implementations. cmd_timer.go wires
// two coupled commands:
//
//   - `hive timer`       : visible parent that validates input, inserts the
//     timers row, forks a detached child, and exits in
//     milliseconds.
//   - `hive timer-fire`  : hidden child that sleeps until fires_at, runs
//     agent-send via exec, runs triage on failure, and
//     marks the row fired/failed.
//
// Both halves share a single Cmd struct so they can register together via
// the standard `New<Foo>Cmd(flags, app).Register(app)` pattern used by every
// other command in this package.
package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/core/timer"
	"github.com/colonyops/hive/internal/core/tmux"
	"github.com/colonyops/hive/internal/data/db"
	"github.com/colonyops/hive/internal/data/stores"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/internal/hive/scripts"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

// timerCapPerSession is the maximum number of active timers a single session
// may schedule without --ignore-limit.
const timerCapPerSession = 3

// timerIDLength is the length of the random ID assigned to each timer row.
// Slightly longer than session IDs (6) because timer IDs are internal and
// we want extra collision headroom for sessions that schedule many timers
// over time.
const timerIDLength = 12

// agentSendTimeout caps how long the child waits for agent-send to complete.
// agent-send itself is a short shell script that returns once tmux send-keys
// has been issued; 5s is generous.
const agentSendTimeout = 5 * time.Second

// TimerCmd is the shared receiver for both `timer` and `timer-fire`.
type TimerCmd struct {
	flags *Flags
	app   *hive.App

	// schedule flags (parent).
	duration    string
	prompt      string
	ignoreLimit bool
	asJSON      bool

	// fire flag (hidden child).
	fireID string
}

// NewTimerCmd constructs the shared timer command receiver.
func NewTimerCmd(flags *Flags, app *hive.App) *TimerCmd {
	return &TimerCmd{flags: flags, app: app}
}

// Register adds both `hive timer` (visible) and `hive timer-fire` (hidden)
// to the application.
func (cmd *TimerCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, cmd.scheduleCmd(), cmd.fireCmd())
	return app
}

func (cmd *TimerCmd) scheduleCmd() *cli.Command {
	return &cli.Command{
		Name:      "timer",
		Usage:     "Schedule a delayed self-prompt for the current session",
		UsageText: `hive timer --duration <d> --prompt <text> [--ignore-limit] [--json]`,
		Description: `Schedules a single delayed prompt that this agent's tmux pane will
receive after <duration> elapses. After scheduling, the parent exits in
milliseconds; a detached child sends the prompt via agent-send. Use as the
one-shot counterpart to /loop.

  --prompt -   reads the prompt from stdin (single read; trims trailing newline).

By default, each session may have up to 3 active timers — use
--ignore-limit to bypass.`,
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			return ctx, cmd.app.FullBootstrap(ctx)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "duration",
				Aliases:     []string{"d"},
				Usage:       "duration (e.g. 5s, 3m, 1h) [1s..4h]",
				Required:    true,
				Destination: &cmd.duration,
			},
			&cli.StringFlag{
				Name:        "prompt",
				Aliases:     []string{"p"},
				Usage:       "text to send (use - to read from stdin)",
				Required:    true,
				Destination: &cmd.prompt,
			},
			&cli.BoolFlag{
				Name:        "ignore-limit",
				Usage:       "bypass the 3-active-timer cap (logs WARN)",
				Destination: &cmd.ignoreLimit,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "emit JSON confirmation instead of text",
				Destination: &cmd.asJSON,
			},
		},
		Action: cmd.runSchedule,
	}
}

func (cmd *TimerCmd) fireCmd() *cli.Command {
	return &cli.Command{
		Name:   "timer-fire",
		Usage:  "internal: fire a previously-scheduled timer (run by hive itself)",
		Hidden: true,
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			return ctx, cmd.app.MinimalBootstrap(ctx)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "id",
				Usage:       "timer row id to fire",
				Required:    true,
				Destination: &cmd.fireID,
			},
		},
		Action: cmd.runFire,
	}
}

func (cmd *TimerCmd) runSchedule(ctx context.Context, c *cli.Command) error {
	d, err := timer.ParseDuration(cmd.duration)
	if err != nil {
		return err
	}

	prompt, err := cmd.resolvePrompt(os.Stdin)
	if err != nil {
		return err
	}
	if err := timer.ValidatePrompt(prompt); err != nil {
		return err
	}

	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		return fmt.Errorf("detect session: %w", err)
	}
	if sessionID == "" {
		return fmt.Errorf("could not detect session from working directory; run 'hive session info' to check")
	}

	target, err := cmd.app.Sessions.ResolveTmuxTarget(ctx, sessionID)
	if err != nil {
		return err
	}

	q := cmd.app.DB.Queries()

	// Schedule-time inactive-marker pass: mark any active timers whose
	// owning PIDs are gone as orphaned before we count toward the cap.
	if n, err := timer.MarkInactiveForSession(ctx, stores.DBTimerAdapter{Q: q}, sessionID); err != nil {
		log.Debug().Err(err).Str("session_id", sessionID).Msg("inactive-marker pass failed (continuing)")
	} else if n > 0 {
		log.Debug().Int("marked", n).Str("session_id", sessionID).Msg("schedule-time inactive marker")
	}

	active, err := q.ActiveTimersForSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("list active timers: %w", err)
	}
	if !cmd.ignoreLimit && len(active) >= timerCapPerSession {
		return fmt.Errorf("session has %d active timers; cap is %d — use --ignore-limit to bypass",
			len(active), timerCapPerSession)
	}
	if cmd.ignoreLimit {
		log.Warn().Str("session_id", sessionID).Int("active", len(active)).Msg("timer cap bypass")
	}

	now := time.Now()
	firesAt := now.Add(d)
	id := randid.Generate(timerIDLength)
	if err := q.InsertTimer(ctx, db.InsertTimerParams{
		ID:         id,
		SessionID:  sessionID,
		TmuxTarget: target,
		Prompt:     prompt,
		DurationNs: int64(d),
		FiresAt:    firesAt.UnixNano(),
		Pid:        sql.NullInt64{},
		Status:     timer.StatusActive,
		CreatedAt:  now.UnixNano(),
		FiredAt:    sql.NullInt64{},
	}); err != nil {
		return fmt.Errorf("insert timer: %w", err)
	}

	self, err := os.Executable()
	if err != nil {
		// Roll back the row so the cap doesn't permanently include a dead entry.
		if delErr := q.DeleteTimer(ctx, id); delErr != nil {
			log.Warn().Err(delErr).Str("timer_id", id).Msg("rollback after os.Executable failed")
		}
		return fmt.Errorf("locate self executable: %w", err)
	}
	childEnv := buildChildEnv(cmd.app.Opts)
	pid, forkErr := timer.ForkChild(self, []string{"timer-fire", "--id", id}, childEnv)
	if pid == 0 {
		// Child did not start — roll back the row so the cap stays accurate.
		if delErr := q.DeleteTimer(ctx, id); delErr != nil {
			log.Warn().Err(delErr).Str("timer_id", id).Msg("rollback after fork failed")
		}
		return fmt.Errorf("fork timer-fire: %w", forkErr)
	}
	if forkErr != nil {
		// Child is running (pid > 0) but Release failed. Log and continue —
		// the row stays and the child will fire normally.
		log.Warn().Err(forkErr).Int("pid", pid).Str("timer_id", id).Msg("fork partial failure (child alive, continuing)")
	}

	if err := q.UpdateTimerPID(ctx, db.UpdateTimerPIDParams{
		Pid: sql.NullInt64{Int64: int64(pid), Valid: true},
		ID:  id,
	}); err != nil {
		// Child is already running; just log. The fire path / sweep will reconcile.
		log.Warn().Err(err).Int("pid", pid).Str("timer_id", id).Msg("update timer pid failed (child running anyway)")
	}

	return cmd.printConfirmation(c.Root().Writer, id, target, firesAt, d, pid)
}

// resolvePrompt returns cmd.prompt unchanged, or reads stdin when the flag
// value is the literal "-". Trailing newlines and carriage returns are trimmed.
func (cmd *TimerCmd) resolvePrompt(stdin io.Reader) (string, error) {
	if cmd.prompt != "-" {
		return cmd.prompt, nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read prompt from stdin: %w", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func (cmd *TimerCmd) printConfirmation(w io.Writer, id, target string, firesAt time.Time, d time.Duration, pid int) error {
	if cmd.asJSON {
		type scheduleResult struct {
			ID        string `json:"id"`
			FiresAt   string `json:"fires_at"`
			InSeconds int64  `json:"in_seconds"`
			Target    string `json:"target"`
			PID       int    `json:"pid"`
		}
		return json.NewEncoder(w).Encode(scheduleResult{
			ID:        id,
			FiresAt:   firesAt.UTC().Format(time.RFC3339),
			InSeconds: int64(d.Seconds()),
			Target:    target,
			PID:       pid,
		})
	}
	_, err := fmt.Fprintf(w, "timer scheduled id=%s fires_at=%s (in %s) target=%s\n",
		id, firesAt.UTC().Format(time.RFC3339), d, target)
	return err
}

// buildChildEnv returns the environment for the detached child. The full
// parent env is inherited so PATH and friends are preserved, then HIVE_*
// vars are injected from the resolved bootstrap opts so the child sees the
// same data dir / config / log file as the parent.
//
// Empty opts fields are left unset — the child inherits the parent's env var
// (or uses its own default), which is the correct "unset means use default"
// behaviour.
func buildChildEnv(opts hive.BootstrapOptions) []string {
	env := append([]string{}, os.Environ()...)
	inject := map[string]string{
		"HIVE_DATA_DIR":  opts.DataDir,
		"HIVE_CONFIG":    opts.ConfigPath,
		"HIVE_LOG_FILE":  opts.LogFile,
		"HIVE_LOG_LEVEL": opts.LogLevel,
	}
	for k, v := range inject {
		if v == "" {
			continue
		}
		env = setEnvKey(env, k, v)
	}
	return env
}

// setEnvKey replaces an existing KEY=value entry in env, or appends it.
func setEnvKey(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func (cmd *TimerCmd) runFire(ctx context.Context, _ *cli.Command) error {
	q := cmd.app.DB.Queries()
	row, err := q.GetTimer(ctx, cmd.fireID)
	if err != nil {
		return fmt.Errorf("load timer row %q: %w", cmd.fireID, err)
	}
	if row.Status != timer.StatusActive {
		// Row was already marked (e.g. orphaned by a sweep). Exit cleanly.
		log.Info().
			Str("timer_id", row.ID).
			Str("status", string(row.Status)).
			Msg("timer already non-active, skipping fire")
		return nil
	}

	sleepDur := time.Until(time.Unix(0, row.FiresAt))
	if sleepDur > 0 {
		t := time.NewTimer(sleepDur)
		defer t.Stop()
		<-t.C
	}

	agentSendPath := scripts.ScriptPaths(cmd.app.Opts.DataDir)["agent-send"]
	if agentSendPath == "" {
		return cmd.recordFailure(ctx, q, row, -1, "agent-send script path not resolved", "")
	}

	sendCtx, cancel := context.WithTimeout(ctx, agentSendTimeout)
	defer cancel()
	execCmd := exec.CommandContext(sendCtx, agentSendPath, row.TmuxTarget, row.Prompt)
	var stderrBuf strings.Builder
	execCmd.Stderr = &stderrBuf
	if execErr := execCmd.Run(); execErr != nil {
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(execErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return cmd.recordFailure(ctx, q, row, exitCode, strings.TrimSpace(stderrBuf.String()), execErr.Error())
	}

	if err := q.MarkTimerFired(ctx, db.MarkTimerFiredParams{
		FiredAt: sql.NullInt64{Int64: time.Now().UnixNano(), Valid: true},
		ID:      row.ID,
	}); err != nil {
		log.Warn().Err(err).Str("timer_id", row.ID).Msg("update fired status failed")
	}
	log.Info().Str("timer_id", row.ID).Str("session_id", row.SessionID).Msg("timer fired")
	return nil
}

// recordFailure runs the tmux triage checks, emits a structured WARN event,
// and marks the row failed. Returns nil — the child reports via the log
// line, not via exit code (a non-zero exit would only surface launch
// errors during manual testing).
func (cmd *TimerCmd) recordFailure(ctx context.Context, q *db.Queries, row db.Timer, exitCode int, stderr, extra string) error {
	tmuxClient := tmux.New(&executil.RealExecutor{}, log.Logger)
	report := runTmuxTriage(ctx, tmuxClient, row.TmuxTarget, exitCode, stderr)

	log.Warn().
		Str("timer_id", row.ID).
		Str("session_id", row.SessionID).
		Int("agent_send_exit", report.agentSendExit).
		Str("agent_send_stderr", report.agentSendStderr).
		Bool("tmux_server_exists", report.tmuxServerExists).
		Bool("session_exists", report.sessionExists).
		Bool("window_exists", report.windowExists).
		Str("extra", extra).
		Msg("timer fire failed")

	if err := q.MarkTimerFailed(ctx, db.MarkTimerFailedParams{
		FiredAt: sql.NullInt64{Int64: time.Now().UnixNano(), Valid: true},
		ID:      row.ID,
	}); err != nil {
		log.Warn().Err(err).Str("timer_id", row.ID).Msg("update failed status failed")
	}
	return nil
}
