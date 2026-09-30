package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal/assess"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
	"github.com/colonyops/hive/internal/hive"
)

// DetectCmd classifies tmux panes for a hive session.
type DetectCmd struct {
	flags *Flags
	app   *hive.App
}

// NewDetectCmd creates a new detect command.
func NewDetectCmd(flags *Flags, app *hive.App) *DetectCmd {
	return &DetectCmd{flags: flags, app: app}
}

// Register adds the detect command to the application.
func (cmd *DetectCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:      "detect",
		Hidden:    true,
		Usage:     "Classify tmux panes for a session (development/diagnostic tool)",
		UsageText: "hive detect <session-slug>",
		Action:    cmd.run,
	})
	return app
}

type detectPaneOutput struct {
	PaneID      string `json:"paneID"`
	PanePID     int64  `json:"panePID"`
	WindowIndex string `json:"windowIndex"`
	PaneIndex   string `json:"paneIndex"`
	WindowName  string `json:"windowName"`
	IsAgent     bool   `json:"isAgent"`
	Tool        string `json:"tool,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
	Tier        int    `json:"tier"`
	InMode      bool   `json:"inMode"`               // from #{pane_in_mode}
	Assessment  string `json:"assessment,omitempty"` // assess.State from a one-shot Engine.Assess
	RuleID      string `json:"ruleID,omitempty"`
}

type detectOutput struct {
	Session string             `json:"session"`
	Panes   []detectPaneOutput `json:"panes"`
}

func (cmd *DetectCmd) run(ctx context.Context, c *cli.Command) error {
	if c.Args().Len() != 1 {
		return fmt.Errorf("usage: hive detect <session-slug>")
	}

	sess, err := cmd.findSession(ctx, c.Args().First())
	if err != nil {
		return err
	}

	if cmd.app.Multiplexer == nil {
		return fmt.Errorf("tmux is unavailable")
	}
	source := cmd.app.Multiplexer
	panes, err := source.ListPanes(ctx)
	if err != nil {
		return err
	}

	tmuxSessions := detectTmuxSessionNames(sess)

	capture := terminaltmux.PaneCapture{Source: source}
	cls := terminaltmux.NewFromPreviewMatchers(cmd.app.Config.Tmux.PreviewWindowMatcher, terminaltmux.WithPaneSource(source)).Classifier()
	engine := assess.NewEngine()
	out := detectOutput{Session: sess.Slug}
	for _, pane := range panes {
		if !tmuxSessions[pane.Target.Session] {
			continue
		}
		input := classifier.InputFromPane(pane)
		result := cls.Classify(ctx, input)
		paneOut := detectPaneOutput{
			PaneID:      pane.NativeID,
			PanePID:     pane.PID,
			WindowIndex: pane.Target.Window,
			PaneIndex:   pane.Target.Pane,
			WindowName:  pane.WindowName,
			IsAgent:     result.IsAgent,
			Tool:        result.Tool,
			Confidence:  string(result.Confidence),
			Tier:        result.Tier,
			InMode:      pane.InMode,
		}
		if result.IsAgent {
			paneOut.Assessment, paneOut.RuleID = assessDetectedAgentPane(ctx, input, result.Tool, capture, engine)
		}
		out.Panes = append(out.Panes, paneOut)
	}

	enc := json.NewEncoder(c.Root().Writer)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func assessDetectedAgentPane(ctx context.Context, pane classifier.PaneInput, tool string, capture classifier.ContentCapture, engine *assess.Engine) (string, string) {
	content, err := capture.CapturePane(ctx, pane.Target)
	if err != nil {
		return "", ""
	}
	if tool == "" {
		tool = "agent"
	}
	assessment := engine.Assess(assess.Snapshot{
		Content: content,
		Title:   pane.PaneTitle,
		Tool:    tool,
		InMode:  pane.InMode,
	})
	return string(assessment.State), assessment.RuleID
}

func detectTmuxSessionNames(sess session.Session) map[string]bool {
	names := make(map[string]bool, 3)
	if metaName := sess.Metadata[session.MetaTmuxSession]; metaName != "" {
		names[metaName] = true
	}
	if sess.Slug != "" {
		names[sess.Slug] = true
	}
	if sess.Name != "" {
		names[sess.Name] = true
	}
	return names
}

func (cmd *DetectCmd) findSession(ctx context.Context, ref string) (session.Session, error) {
	sessions, err := cmd.app.Sessions.ListSessions(ctx)
	if err != nil {
		return session.Session{}, fmt.Errorf("listing sessions: %w", err)
	}
	for _, sess := range sessions {
		if sess.ID == ref || sess.Slug == ref || sess.Name == ref {
			return sess, nil
		}
	}
	return session.Session{}, fmt.Errorf("session %q not found", ref)
}
