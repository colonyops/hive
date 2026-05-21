package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/core/terminal/classifier"
	"github.com/colonyops/hive/internal/core/terminal/content"
	"github.com/colonyops/hive/internal/core/terminal/process"
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
		Usage:     "Classify tmux panes for a session",
		UsageText: "hive detect <session-slug>",
		Action:    cmd.run,
	})
	return app
}

type detectPaneOutput struct {
	PaneID      string `json:"paneID"`
	PanePID     int64  `json:"panePID"`
	WindowIndex string `json:"windowIndex"`
	WindowName  string `json:"windowName"`
	IsAgent     bool   `json:"isAgent"`
	Tool        string `json:"tool,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
	Tier        int    `json:"tier"`
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

	lister := terminaltmux.TmuxPaneLister{}
	panes, err := lister.ListAllPanes()
	if err != nil {
		return err
	}

	tmuxSessions := detectTmuxSessionNames(sess)

	titlePatterns := classifier.TitlePatternsFromConfig(cmd.app.Config.Tmux.PreviewWindowMatcher)
	cls := classifier.New(titlePatterns, process.OSReader{}, terminaltmux.TmuxCapture{}, content.NewScorer())
	out := detectOutput{Session: sess.Slug}
	for _, pane := range panes {
		if !tmuxSessions[pane.SessionName] {
			continue
		}
		result := cls.Classify(ctx, pane)
		out.Panes = append(out.Panes, detectPaneOutput{
			PaneID:      pane.PaneID,
			PanePID:     pane.PanePID,
			WindowIndex: pane.WindowIndex,
			WindowName:  pane.WindowName,
			IsAgent:     result.IsAgent,
			Tool:        result.Tool,
			Confidence:  string(result.Confidence),
			Tier:        result.Tier,
		})
	}

	enc := json.NewEncoder(c.Root().Writer)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
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
