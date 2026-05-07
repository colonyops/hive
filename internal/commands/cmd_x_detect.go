package commands

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/core/terminal"
	terminaltmux "github.com/colonyops/hive/internal/core/terminal/tmux"
)

func (cmd *ExperimentalCmd) detectCmd() *cli.Command {
	return &cli.Command{
		Name:  "detect",
		Usage: "Diagnose agent detection across all tmux panes",
		Action: func(ctx context.Context, c *cli.Command) error {
			return cmd.runDetect(ctx, c)
		},
	}
}

func (cmd *ExperimentalCmd) runDetect(ctx context.Context, c *cli.Command) error {
	tmuxI := terminaltmux.New(cmd.app.Config.Tmux.PreviewWindowMatcher)
	if !tmuxI.Available() {
		_, _ = fmt.Fprintln(c.Writer, "tmux not available")
		return nil
	}
	disc := terminal.AllPanesDiscoverer(tmuxI)

	panes, err := disc.DiscoverAllPanes(ctx)
	if err != nil {
		return fmt.Errorf("discover panes: %w", err)
	}

	if len(panes) == 0 {
		_, _ = fmt.Fprintln(c.Writer, "no tmux panes found")
		return nil
	}

	printPaneTree(c.Writer, panes)
	return nil
}

// printPaneTree renders panes grouped by tmux session then by window.
func printPaneTree(w io.Writer, panes []terminal.PaneDetail) {
	// Group: session → window index → panes
	type windowKey struct{ session, windowIndex string }
	windowPanes := make(map[windowKey][]terminal.PaneDetail)
	windowNames := make(map[windowKey]string)
	var sessionOrder []string
	sessionSeen := make(map[string]bool)

	for _, p := range panes {
		key := windowKey{p.TmuxSession, p.WindowIndex}
		windowPanes[key] = append(windowPanes[key], p)
		windowNames[key] = p.WindowName
		if !sessionSeen[p.TmuxSession] {
			sessionSeen[p.TmuxSession] = true
			sessionOrder = append(sessionOrder, p.TmuxSession)
		}
	}
	sort.Strings(sessionOrder)

	for _, sess := range sessionOrder {
		_, _ = fmt.Fprintf(w, "%s\n", sess)

		// Collect windows for this session, sorted by index
		var keys []windowKey
		for k := range windowPanes {
			if k.session == sess {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].windowIndex < keys[j].windowIndex })

		for ki, wk := range keys {
			isLastWindow := ki == len(keys)-1
			winConnector := "├─"
			if isLastWindow {
				winConnector = "└─"
			}
			_, _ = fmt.Fprintf(w, "  %s window %s (%s)\n", winConnector, wk.windowIndex, windowNames[wk])

			panePfx := "│ "
			if isLastWindow {
				panePfx = "  "
			}

			wPanes := windowPanes[wk]
			for pi, p := range wPanes {
				isLastPane := pi == len(wPanes)-1
				paneConnector := "├─"
				if isLastPane {
					paneConnector = "└─"
				}

				hive := p.HiveSession
				if hive == "" {
					hive = "(none)"
				}
				tool := p.Tool
				if tool == "" {
					tool = "?"
				}
				fgPID := "-"
				if p.FgPID > 0 {
					fgPID = fmt.Sprintf("%d", p.FgPID)
				}
				panePID := "-"
				if p.PanePID > 0 {
					panePID = fmt.Sprintf("%d", p.PanePID)
				}

				_, _ = fmt.Fprintf(w, "  %s  %s pane %s  hive:%-16s  pid:%-7s  fg:%-7s  tool:%s\n",
					panePfx, paneConnector, p.PaneID, hive, panePID, fgPID, tool)
			}
		}
	}
}
