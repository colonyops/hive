package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/status"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
	"github.com/colonyops/hive/internal/hive"
)

// tmuxSender sends real input to a tmux pane. A production sender talks to
// the real tmux binary (realTmuxSender); scenario tests inject a fake so a
// scenario run is never exercised against a live pane in this repo.
type tmuxSender interface {
	SendText(ctx context.Context, target, text string) error
	SendKey(ctx context.Context, target, key string) error
}

// realTmuxSender is the production tmuxSender.
type realTmuxSender struct{}

// SendText types text literally (-l, so nothing in it is ever read as a
// tmux key name) followed by a separate Enter — this is `send`'s wire
// semantics ("send-keys text + Enter" per the scenario format).
func (realTmuxSender) SendText(ctx context.Context, target, text string) error {
	if err := exec.CommandContext(ctx, "tmux", "send-keys", "-t", target, "-l", "--", text).Run(); err != nil {
		return fmt.Errorf("tmux send-keys (text): %w", err)
	}
	if err := exec.CommandContext(ctx, "tmux", "send-keys", "-t", target, "Enter").Run(); err != nil {
		return fmt.Errorf("tmux send-keys (Enter): %w", err)
	}
	return nil
}

// SendKey sends one named key (tmux's own key-name vocabulary: "Down",
// "Enter", "C-c", ...) — this is `key`'s wire semantics.
func (realTmuxSender) SendKey(ctx context.Context, target, key string) error {
	if err := exec.CommandContext(ctx, "tmux", "send-keys", "-t", target, key).Run(); err != nil {
		return fmt.Errorf("tmux send-keys (key %s): %w", key, err)
	}
	return nil
}

// assessScenarioCmd registers `hive x assess scenario`: it drives a tmux
// pane through a scripted YAML scenario while simultaneously observing it
// through the same capture->assess->tracker pipeline `watch` uses, then
// scores the resulting observation log and prints a JSON report.
//
// Sends real input (send-keys) — see ensureContainerSafe: this refuses to
// run against the host's default tmux socket unless --allow-host is passed.
func (cmd *ExperimentalCmd) assessScenarioCmd() *cli.Command {
	var (
		flagTarget    string
		flagAllowHost bool
		flagInterval  time.Duration
	)

	return &cli.Command{
		Name:      "scenario",
		Usage:     "Drive and score a scripted scenario against a tmux pane (sends input; container-only)",
		UsageText: "hive x assess scenario <scenario.yaml> --target <pane> [--allow-host] [--interval 1.5s]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "target",
				Usage:       "tmux pane target to drive and observe (e.g. session:0.0)",
				Required:    true,
				Destination: &flagTarget,
			},
			&cli.BoolFlag{
				Name:        "allow-host",
				Usage:       "Bypass the host-tmux safety interlock (deliberate, eyes-open exceptions only)",
				Destination: &flagAllowHost,
			},
			&cli.DurationFlag{
				Name:        "interval",
				Usage:       "Poll interval while awaiting an expectation (default: terminal.status config, falling back to tmux.poll_interval)",
				Destination: &flagInterval,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.Args().Len() != 1 {
				return fmt.Errorf("usage: hive x assess scenario <scenario.yaml> --target <pane> [--allow-host] [--interval 1.5s]")
			}

			return runAssessScenarioCmd(ctx, c.Root().Writer, c.Args().First(), flagTarget, flagAllowHost, flagInterval, cmd.app, resolveTmuxSocketPath)
		},
	}
}

// runAssessScenarioCmd is assessScenarioCmd's Action, minus the cli.Command
// plumbing: resolve is injected so the safety interlock is testable without
// a real tmux server (see assess_safety_test.go's pattern). Nothing past
// the interlock check is covered by this repo's own tests — the scorer is
// the unit-tested surface (assess_scenario_test.go); this function's tmux
// interaction is exercised only by hand, inside `mise container`.
func runAssessScenarioCmd(ctx context.Context, w io.Writer, scenarioPath, target string, allowHost bool, interval time.Duration, app *hive.App, resolve socketPathResolver) error {
	if err := ensureContainerSafe(ctx, target, allowHost, resolve); err != nil {
		return err
	}

	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		return fmt.Errorf("reading scenario file: %w", err)
	}

	spec, err := parseScenario(data)
	if err != nil {
		return err
	}

	opts := status.DefaultOptions()
	if app != nil && app.Config != nil {
		opts = status.OptionsFromConfig(app.Config.Terminal.Status, app.Config.Tmux.PollInterval)
	}
	if interval <= 0 {
		interval = opts.PollInterval
	}

	tracker := status.NewTracker(assess.NewEngine(), opts)
	log, runErr := runScenario(ctx, target, spec, realTmuxSender{}, terminaltmux.TmuxCapture{}, tracker, interval)
	report := scoreScenario(spec, log)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(report); encErr != nil {
		return encErr
	}

	if runErr != nil {
		return fmt.Errorf("scenario run ended early: %w", runErr)
	}
	return nil
}

// runScenario executes spec's steps in order against target, using sender
// for send/key steps and capture+tracker to observe published status for
// expect steps. It always returns whatever poll log it managed to collect,
// even on error, so a mid-run failure still yields a partial report.
//
// Only `expect` steps consume polls: send/key steps act immediately with no
// wait. This means the log is a concatenation of contiguous per-expect-step
// windows with no gaps — exactly the shape scoreScenario's pollsForStep
// assumes. For each expect step, the loop first polls up to WithinPolls
// times searching for the target state; once found, it polls
// (HoldsForPolls-1) more times to build the hold-confirmation tail that
// scoreScenario checks. If the state is never found, the window is exactly
// WithinPolls polls long and the loop moves on — there is nothing to hold.
func runScenario(ctx context.Context, target string, spec *scenarioSpec, sender tmuxSender, capture terminaltmux.TmuxCapture, tracker *status.Tracker, interval time.Duration) ([]scenarioPoll, error) {
	var (
		log        []scenarioPoll
		pollSeq    int
		generation uint64
	)

	pollOnce := func(stepIdx int) error {
		content, err := capture.CapturePane(ctx, target)
		if err != nil {
			return fmt.Errorf("capture-pane: %w", err)
		}
		title, inMode, err := paneExtra(ctx, target)
		if err != nil {
			return fmt.Errorf("display-message: %w", err)
		}

		generation++
		tool := spec.Tool
		if tool == "" {
			tool = terminal.DetectTool(content)
		}
		snap := assess.Snapshot{Content: content, Title: title, Tool: tool, InMode: inMode, Generation: generation}
		published, _ := tracker.Observe(target, snap)

		log = append(log, scenarioPoll{Poll: pollSeq, Published: published, StepIndex: stepIdx})
		pollSeq++
		return nil
	}

	wait := func() error {
		t := time.NewTimer(interval)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}

	for stepIdx, step := range spec.Steps {
		switch step.Kind {
		case scenarioStepSend:
			if err := sender.SendText(ctx, target, step.Send); err != nil {
				return log, fmt.Errorf("step %d (send): %w", stepIdx, err)
			}
		case scenarioStepKey:
			if err := sender.SendKey(ctx, target, step.Key); err != nil {
				return log, fmt.Errorf("step %d (key): %w", stepIdx, err)
			}
		case scenarioStepExpect:
			detectedAt, err := awaitExpectation(ctx, stepIdx, step.Expect, wait, pollOnce, &log)
			if err != nil {
				return log, fmt.Errorf("step %d (expect): %w", stepIdx, err)
			}
			if detectedAt >= 0 {
				for i := 1; i < step.Expect.HoldsForPolls; i++ {
					if err := wait(); err != nil {
						return log, fmt.Errorf("step %d (expect, hold confirmation): %w", stepIdx, err)
					}
					if err := pollOnce(stepIdx); err != nil {
						return log, fmt.Errorf("step %d (expect, hold confirmation): %w", stepIdx, err)
					}
				}
			}
		}
	}

	return log, nil
}

// awaitExpectation runs the search phase of one expect step: poll up to
// exp.WithinPolls times, returning the 0-based offset of the first poll
// whose published status matched (or -1 if the window was exhausted with no
// match).
func awaitExpectation(_ context.Context, stepIdx int, exp scenarioExpect, wait func() error, pollOnce func(int) error, log *[]scenarioPoll) (int, error) {
	for i := 0; i < exp.WithinPolls; i++ {
		if err := wait(); err != nil {
			return -1, err
		}
		if err := pollOnce(stepIdx); err != nil {
			return -1, err
		}
		if (*log)[len(*log)-1].Published == exp.State {
			return i, nil
		}
	}
	return -1, nil
}
