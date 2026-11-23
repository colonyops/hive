package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/colonyops/hive/pkg/logutils"
	"github.com/rs/zerolog"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/cmd/hive/internal/styles"
	"github.com/colonyops/hive/cmd/hive/internal/tui"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/timeutil"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

// HoneycombCmd implements the hive hc command group.
type HoneycombCmd struct {
	flags *Flags
	app   *app.App
}

// NewHoneycombCmd creates a new hc command.
func NewHoneycombCmd(flags *Flags, app *app.App) *HoneycombCmd {
	return &HoneycombCmd{flags: flags, app: app}
}

// Register adds the hc command to the application.
func (cmd *HoneycombCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "hc",
		Usage: "Track tasks and epics for agent workflows",
		Description: `hc (Honeycomb) is a task tracking system for LLM agents — like GitHub Issues,
but scoped to a repository and designed for machine consumption.

Run it with no subcommand to open the task browser. The subcommands are the
tasks operations of the catalog, the same ones Hive Desktop serves over MCP.
They print JSON Lines; list, get and context get print a tree, a styled item
or markdown when stdout is a terminal.

The caller and the repository come from --caller, else $HIVE_AGENT_SESSION,
else the working directory.`,
		Action: cmd.run,
	})
	return app
}

func (cmd *HoneycombCmd) run(ctx context.Context, c *cli.Command) error {
	opts := tui.HoneycombOnlyOptions{
		Honeycomb: cmd.app.HC(),
		RepoKey:   cmd.detectRepoKey(ctx),
		Config:    cmd.app.Config,
		KVStore:   cmd.app.KV(),
		Renderer:  cmd.app.Renderer(),
		Logger:    cmd.app.Logger,
	}
	m := tui.NewHoneycombOnly(opts)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		return fmt.Errorf("run hc TUI: %w", err)
	}
	return nil
}

func (cmd *HoneycombCmd) logger() *zerolog.Logger {
	logger := logutils.Component(cmd.app.Logger, "cli.hc")
	return &logger
}

func (cmd *HoneycombCmd) detectRepoKey(ctx context.Context) string {
	url, err := cmd.app.Sessions().Git().RemoteURL(ctx, ".")
	if err != nil {
		cmd.logger().Debug().Err(err).Msg("failed to get remote URL for hc")
		return ""
	}
	owner, repoName := git.ExtractOwnerRepo(url)
	if owner == "" || repoName == "" {
		return ""
	}
	return owner + "/" + repoName
}

type treeNode struct {
	item     hc.Item
	children []*treeNode
}

// statusSymbol returns a styled status indicator for the given status.
func statusSymbol(status hc.Status) string {
	switch status {
	case hc.StatusOpen:
		return styles.TextForegroundStyle.Render("○")
	case hc.StatusInProgress:
		return styles.TextPrimaryStyle.Render("◉")
	case hc.StatusDone:
		return lipgloss.NewStyle().Foreground(styles.ColorSuccess).Faint(true).Render("✓")
	case hc.StatusCancelled:
		return styles.TextMutedStyle.Render("✗")
	default:
		return "?"
	}
}

// styledStatus returns the status string rendered with an appropriate color.
func styledStatus(status hc.Status) string {
	switch status {
	case hc.StatusOpen:
		return styles.TextForegroundStyle.Render(string(status))
	case hc.StatusInProgress:
		return styles.TextPrimaryStyle.Render(string(status))
	case hc.StatusDone:
		return styles.TextSuccessStyle.Render(string(status))
	case hc.StatusCancelled:
		return styles.TextMutedStyle.Render(string(status))
	default:
		return string(status)
	}
}

// styledTitle returns the item title rendered with status-appropriate styling.
func styledTitle(item hc.Item) string {
	switch item.Status {
	case hc.StatusOpen:
		if item.IsEpic() {
			return styles.TextForegroundBoldStyle.Render(item.Title)
		}
		return styles.TextForegroundStyle.Render(item.Title)
	case hc.StatusInProgress:
		if item.IsEpic() {
			return styles.TextForegroundBoldStyle.Render(item.Title)
		}
		return styles.TextPrimaryStyle.Render(item.Title)
	case hc.StatusDone, hc.StatusCancelled:
		return styles.TextMutedStyle.Render(item.Title)
	default:
		return item.Title
	}
}

// renderColoredTree prints items as a colored tree with box-drawing connectors.
func renderColoredTree(w io.Writer, items []hc.Item) {
	byID := make(map[string]*treeNode, len(items))
	for i := range items {
		byID[items[i].ID] = &treeNode{item: items[i]}
	}

	var roots []*treeNode
	for i := range items {
		node := byID[items[i].ID]
		if parent, ok := byID[items[i].ParentID]; ok {
			parent.children = append(parent.children, node)
		} else {
			roots = append(roots, node)
		}
	}

	mutedStyle := styles.TextMutedStyle
	assignStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted).Italic(true)
	warningStyle := styles.TextWarningStyle

	var walk func(node *treeNode, prefix, connector string)
	walk = func(node *treeNode, prefix, connector string) {
		item := node.item

		symbol := statusSymbol(item.Status)
		title := styledTitle(item)
		id := mutedStyle.Render("(" + item.ID + ")")

		var extras []string
		if item.SessionID != "" {
			extras = append(extras, assignStyle.Render("→ "+item.SessionID))
		}
		if item.Blocked {
			extras = append(extras, warningStyle.Render("[blocked]"))
		}
		if len(item.Tags) > 0 {
			extras = append(extras, mutedStyle.Render(formatTags(item.Tags)))
		}

		line := prefix + mutedStyle.Render(connector) + symbol + " " + title + " " + id
		if len(extras) > 0 {
			line += "  " + strings.Join(extras, " ")
		}
		_, _ = fmt.Fprintln(w, line)

		// Continuation bar: only if current node is not the last child (connector != "└─ ")
		childPrefix := prefix + "│  "
		if connector == "└─ " || connector == "" {
			childPrefix = prefix + "   "
		}

		for i, child := range node.children {
			childConnector := "├─ "
			if i == len(node.children)-1 {
				childConnector = "└─ "
			}
			walk(child, childPrefix, childConnector)
		}
	}

	for _, root := range roots {
		walk(root, "", "")
	}
}

func formatTags(tags []string) string {
	return "#" + strings.Join(tags, " #")
}

// renderMarkdown renders markdown to w using glamour when w is a TTY,
// falling back to plain text otherwise.
func renderMarkdown(w io.Writer, markdown string) {
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if r, err := glamour.NewTermRenderer(glamour.WithEnvironmentConfig()); err == nil {
			if rendered, err := r.Render(markdown); err == nil {
				_, _ = fmt.Fprint(w, rendered)
				return
			}
		}
	}
	_, _ = fmt.Fprint(w, markdown)
}

// terminalWidth returns the terminal width from w if it's a TTY, or 80 as a default.
func terminalWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if width, _, err := term.GetSize(int(f.Fd())); err == nil && width > 0 {
			return width
		}
	}
	return 80
}

// renderItem prints a styled detail view of an item and its comments.
func renderItem(w io.Writer, item hc.Item, comments []hc.Comment, epicTitle string, blockers []hc.Item) {
	mutedStyle := styles.TextMutedStyle
	fgStyle := styles.TextForegroundStyle
	indent := "  "
	indentLen := len(indent)
	tWidth := terminalWidth(w)
	if tWidth > 90 {
		tWidth = 90
	}

	// Header: symbol + title + (id)
	symbol := statusSymbol(item.Status)
	title := styledTitle(item)
	_, _ = fmt.Fprintf(w, "%s %s %s\n", symbol, title, mutedStyle.Render("("+item.ID+")"))

	// Second line: type · status [→ session]
	statusText := styledStatus(item.Status)
	statusLine := mutedStyle.Render(string(item.Type)) + " · " + statusText
	if item.SessionID != "" {
		assignStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted).Italic(true)
		statusLine += " " + assignStyle.Render("→ "+item.SessionID)
	}
	_, _ = fmt.Fprintf(w, "%s%s\n", indent, statusLine)
	if len(item.Tags) > 0 {
		_, _ = fmt.Fprintf(w, "%s%s\n", indent, mutedStyle.Render(formatTags(item.Tags)))
	}

	// Epic breadcrumb
	if epicTitle != "" {
		_, _ = fmt.Fprintln(w)
		epicBadge := lipgloss.NewStyle().
			Background(styles.ColorSurface).
			Foreground(styles.ColorForeground).
			Bold(true).
			PaddingRight(1).
			Render("Epic")
		_, _ = fmt.Fprintf(w, "%s%s %s\n", indent,
			epicBadge,
			mutedStyle.Render(epicTitle+" ("+item.EpicID+")"))
	}

	// Section divider helper: label is styled foreground, dashes fill to fixed total width
	divider := func(label string) string {
		const totalWidth = 54
		dashCount := totalWidth - len(label) - 1 // -1 for the space
		if dashCount < 4 {
			dashCount = 4
		}
		return fgStyle.Render(label) + " " + mutedStyle.Render(strings.Repeat("─", dashCount))
	}

	// Description
	if item.Desc != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s%s\n", indent, divider("Description"))
		_, _ = fmt.Fprintln(w)
		contentWidth := tWidth - indentLen
		wrapped := lipgloss.Wrap(item.Desc, contentWidth, "")
		for _, line := range strings.Split(wrapped, "\n") {
			_, _ = fmt.Fprintf(w, "%s%s\n", indent, line)
		}
	}

	// Blockers
	if len(blockers) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s%s\n", indent, divider(fmt.Sprintf("Blockers (%d)", len(blockers))))
		_, _ = fmt.Fprintln(w)
		for _, b := range blockers {
			symbol := statusSymbol(b.Status)
			title := styledTitle(b)
			id := mutedStyle.Render("(" + b.ID + ")")
			_, _ = fmt.Fprintf(w, "%s%s %s %s\n", indent, symbol, title, id)
		}
	}

	// Comments
	if len(comments) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s%s\n", indent, divider(fmt.Sprintf("Comments (%d)", len(comments))))

		for _, c := range comments {
			_, _ = fmt.Fprintln(w)

			ts := timeutil.Ago(c.CreatedAt)
			isCheckpoint := strings.HasPrefix(c.Message, "CHECKPOINT")

			// Thread header: ┌─ <time> [CHECKPOINT]
			if isCheckpoint {
				_, _ = fmt.Fprintf(w, "%s%s %s  %s\n", indent,
					mutedStyle.Render("┌─"),
					mutedStyle.Render(ts),
					styles.TextWarningStyle.Render("CHECKPOINT"))
			} else {
				_, _ = fmt.Fprintf(w, "%s%s\n", indent, mutedStyle.Render("┌─ "+ts))
			}

			// Thread body: │  <message lines>
			pipe := mutedStyle.Render("│")
			pipePrefix := indent + pipe + "  "
			// "│  " is 3 visual chars
			bodyWidth := tWidth - indentLen - 3
			wrapped := lipgloss.Wrap(c.Message, bodyWidth, "")
			for _, line := range strings.Split(wrapped, "\n") {
				_, _ = fmt.Fprintf(w, "%s%s\n", pipePrefix, fgStyle.Render(line))
			}
		}
	}
}
