package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/validate"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/colonyops/hive/pkg/logutils"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/hay-kot/criterio"
	"github.com/urfave/cli/v3"
)

type BatchCmd struct {
	flags *Flags
	app   *hive.App
	fr    *iojson.FileReader[BatchInput]
	agent string
}

func NewBatchCmd(flags *Flags, app *hive.App) *BatchCmd {
	return &BatchCmd{
		flags: flags,
		app:   app,
		fr:    &iojson.FileReader[BatchInput]{},
	}
}

func (cmd *BatchCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "batch",
		Usage: "Create multiple sessions from JSON input",
		UsageText: `hive batch [options]

Read from stdin:
  echo '{"sessions":[{"name":"task1","prompt":"Do something"}]}' | hive batch

Read from file:
  hive batch -f sessions.json`,
		Description: `Creates multiple agent sessions from a JSON specification.

Each session in the input array is created sequentially. A terminal is
spawned for each session using the batch_spawn commands if configured,
otherwise falls back to spawn commands.

Processing stops after 3 failures. Sessions not attempted are marked as skipped.

Input JSON schema:
  {
    "sessions": [
      {
        "name": "session-name",
        "session_id": "optional-id",
        "prompt": "optional task prompt",
        "remote": "optional-url",
        "source": "optional-path",
        "agent": "optional-profile"
      }
    ]
  }

Fields:
  name       - Required. Session name (used in path).
  session_id - Optional. Session ID (lowercase alphanumeric, auto-generated if empty).
  prompt     - Optional. Task prompt passed to batch_spawn via {{.Prompt}} template.
  remote     - Optional. Git remote URL (auto-detected from current dir if empty).
  source     - Optional. Directory to copy files from (per copy rules in config).
  agent      - Optional. Agent profile name (overrides --agent default for that session).

Config example (in ~/.config/hive/config.yaml):
  commands:
    spawn:        # Used by hive new
      - "wezterm cli spawn --cwd {{.Path}}"
    batch_spawn:  # Used by hive batch (supports {{.Prompt}})
      - "wezterm cli spawn --cwd {{.Path}} -- claude --prompt '{{.Prompt}}'"

Output is JSON with a batch ID, log file path, and results for each session.`,
		Flags: []cli.Flag{
			cmd.fr.Flag(),
			&cli.StringFlag{
				Name:        "agent",
				Aliases:     []string{"a"},
				Usage:       "default agent profile for all sessions",
				Destination: &cmd.agent,
			},
		},
		Action: cmd.run,
	})

	return app
}

func (cmd *BatchCmd) run(ctx context.Context, c *cli.Command) error {
	batchID := randid.Generate(6)

	logger, closer, err := logutils.New(
		cmd.flags.LogLevel,
		filepath.Join(cmd.app.Config.LogsDir(), "batch-"+batchID+".log"),
	)
	if err != nil {
		return iojson.WriteError(fmt.Sprintf("setup logger: %s", err), nil)
	}
	defer closer()

	logger.Info().Str("batch_id", batchID).Msg("starting batch processing")

	input, err := cmd.fr.Read()
	if err != nil {
		logger.Error().Err(err).Msg("failed to read input")
		return iojson.WriteError(fmt.Sprintf("read input: %s", err), nil)
	}

	if cmd.agent != "" {
		if _, ok := cmd.app.Config.Agents.Profiles[cmd.agent]; !ok {
			return iojson.WriteError(fmt.Sprintf("unknown agent profile %q (available: %s)", cmd.agent, strings.Join(agentProfileNames(cmd.app.Config), ", ")), nil)
		}
	}

	if err := input.ValidateWithProfiles(cmd.app.Config.Agents.Profiles); err != nil {
		logger.Error().Err(err).Msg("input validation failed")
		return iojson.WriteError(fmt.Sprintf("invalid input: %s", err), nil)
	}

	output := BatchOutput{
		BatchID: batchID,
		LogFile: filepath.Join(cmd.app.Config.LogsDir(), fmt.Sprintf("batch-%s.log", batchID)),
		Results: make([]BatchResult, 0, len(input.Sessions)),
	}

	failures := 0
	for i, sess := range input.Sessions {
		if failures >= maxFailures {
			logger.Warn().Str("name", sess.Name).Msg("skipping session due to failure threshold")
			for j := i; j < len(input.Sessions); j++ {
				output.Results = append(output.Results, BatchResult{
					Name:   input.Sessions[j].Name,
					Status: StatusSkipped,
				})
			}
			break
		}

		logger.Info().Str("name", sess.Name).Int("index", i).Msg("creating session")

		result := cmd.createSession(ctx, sess)
		output.Results = append(output.Results, result)

		if result.Status == StatusFailed {
			failures++
			logger.Error().Str("name", sess.Name).Str("error", result.Error).Msg("session creation failed")
		} else {
			logger.Info().Str("name", sess.Name).Str("session_id", result.SessionID).Msg("session created")
		}
	}

	logger.Info().
		Int("total", len(input.Sessions)).
		Int("created", countByStatus(output.Results, StatusCreated)).
		Int("failed", countByStatus(output.Results, StatusFailed)).
		Int("skipped", countByStatus(output.Results, StatusSkipped)).
		Msg("batch processing complete")

	return iojson.Write(output)
}

func (cmd *BatchCmd) createSession(ctx context.Context, sess BatchSession) BatchResult {
	source := sess.Source
	if source == "" {
		var err error
		source, err = os.Getwd()
		if err != nil {
			return BatchResult{
				Name:   sess.Name,
				Status: StatusFailed,
				Error:  fmt.Errorf("determine source directory: %w", err).Error(),
			}
		}
	}

	opts := hive.CreateOptions{
		Name:          sess.Name,
		SessionID:     sess.SessionID,
		Prompt:        sess.Prompt,
		Remote:        sess.Remote,
		Source:        source,
		Agent:         cmd.resolveAgent(sess),
		UseBatchSpawn: true,
	}

	created, err := cmd.app.Sessions.CreateSession(ctx, opts)
	if err != nil {
		return BatchResult{
			Name:   sess.Name,
			Status: StatusFailed,
			Error:  err.Error(),
		}
	}

	return BatchResult{
		Name:      sess.Name,
		SessionID: created.ID,
		Path:      created.Path,
		Status:    StatusCreated,
	}
}

const (
	StatusCreated = "created" // StatusCreated indicates the session was created successfully.
	StatusFailed  = "failed"  // StatusFailed indicates the session creation failed.
	StatusSkipped = "skipped" // StatusSkipped indicates the session was not attempted due to failure threshold.
	maxFailures   = 3         // maxFailures is the number of failures before stopping batch processing.
)

// BatchInput is the JSON input schema for batch session creation.
type BatchInput struct {
	Sessions []BatchSession `json:"sessions"`
}

// Validate checks the batch input for errors using criterio.
func (b BatchInput) Validate() error {
	return b.ValidateWithProfiles(nil)
}

// ValidateWithProfiles checks the batch input and optionally validates
// session agent profile names against known profiles.
func (b BatchInput) ValidateWithProfiles(profiles map[string]config.AgentProfile) error {
	if len(b.Sessions) == 0 {
		return criterio.NewFieldErrors("sessions", fmt.Errorf("array is empty"))
	}

	var errs criterio.FieldErrorsBuilder
	var (
		seenNames = make(map[string]bool)
		seenIDs   = make(map[string]bool)
	)

	for i, sess := range b.Sessions {
		field := fmt.Sprintf("sessions[%d]", i)

		if err := validate.SessionName(sess.Name); err != nil {
			errs = errs.Append(field+".name", err)
			continue
		}

		if seenNames[sess.Name] {
			errs = errs.Append(field+".name", fmt.Errorf("duplicate name %q", sess.Name))
			continue
		}
		seenNames[sess.Name] = true

		// Validate session_id if provided
		if sess.SessionID != "" {
			if err := validate.SessionID(sess.SessionID); err != nil {
				errs = errs.Append(field+".session_id", err)
				continue
			}
			if seenIDs[sess.SessionID] {
				errs = errs.Append(field+".session_id", fmt.Errorf("duplicate session_id %q", sess.SessionID))
				continue
			}
			seenIDs[sess.SessionID] = true
		}

		if sess.Agent != "" && profiles != nil {
			if _, ok := profiles[sess.Agent]; !ok {
				errs = errs.Append(field+".agent", fmt.Errorf("unknown agent profile %q", sess.Agent))
			}
		}
	}

	return errs.ToError()
}

// BatchSession defines a single session to create.
type BatchSession struct {
	Name      string `json:"name"`
	SessionID string `json:"session_id,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Remote    string `json:"remote,omitempty"`
	Source    string `json:"source,omitempty"`
	Agent     string `json:"agent,omitempty"`
}

func (cmd *BatchCmd) resolveAgent(sess BatchSession) string {
	if sess.Agent != "" {
		return sess.Agent
	}
	return cmd.agent
}

// BatchResult is the output for a single session creation attempt.
type BatchResult struct {
	Name      string `json:"name"`
	SessionID string `json:"session_id,omitempty"`
	Path      string `json:"path,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// BatchOutput is the JSON output schema.
type BatchOutput struct {
	BatchID string        `json:"batch_id"`
	LogFile string        `json:"log_file"`
	Results []BatchResult `json:"results"`
}

// BatchErrorOutput is the JSON output for fatal errors.
type BatchErrorOutput struct {
	Error string `json:"error"`
}

func countByStatus(results []BatchResult, status string) int {
	count := 0
	for _, r := range results {
		if r.Status == status {
			count++
		}
	}
	return count
}
