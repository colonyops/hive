package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/hc"
	"github.com/colonyops/hive/internal/core/styles"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/colonyops/hive/pkg/timeutil"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

// HoneycombCmd implements the hive hc command group.
type HoneycombCmd struct {
	flags *Flags
	app   *hive.App
}

// NewHoneycombCmd creates a new hc command.
func NewHoneycombCmd(flags *Flags, app *hive.App) *HoneycombCmd {
	return &HoneycombCmd{flags: flags, app: app}
}

// Register adds the hc command to the application.
func (cmd *HoneycombCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "hc",
		Usage: "Track tasks and epics for agent workflows",
		Description: `hc (Honeycomb) is a task tracking system for LLM agents — like GitHub Issues,
but scoped to a repository and designed for machine consumption.

Session ID and repo key are auto-detected from the working directory.`,
		Commands: []*cli.Command{
			cmd.createCmd(),
			cmd.listCmd(),
			cmd.showCmd(),
			cmd.updateCmd(),
			cmd.nextCmd(),
			cmd.commentCmd(),
			cmd.contextCmd(),
			cmd.pruneCmd(),
		},
	})
	return app
}

func (cmd *HoneycombCmd) detectSession(ctx context.Context) string {
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("failed to detect session for hc")
	}
	return sessionID
}

func (cmd *HoneycombCmd) detectRepoKey(ctx context.Context) string {
	url, err := cmd.app.Sessions.Git().RemoteURL(ctx, ".")
	if err != nil {
		log.Debug().Err(err).Msg("failed to get remote URL for hc")
		return ""
	}
	owner, repoName := git.ExtractOwnerRepo(url)
	if owner == "" || repoName == "" {
		return ""
	}
	return owner + "/" + repoName
}

func (cmd *HoneycombCmd) createCmd() *cli.Command {
	var (
		flagType     string
		flagDesc     string
		flagParentID string
	)
	bulk := iojson.FileReader[hc.CreateInput]{}
	return &cli.Command{
		Name:      "create",
		Aliases:   []string{"new", "add"},
		Usage:     "Create a task or epic",
		UsageText: "hive hc create [title] [--type epic|task] [--desc <desc>] [--parent <id>]",
		Description: `Creates a single item from flags, or a bulk tree from JSON (--file or stdin).

Tasks can nest under other tasks using "children" to create subtrees. The "next"
command walks the tree and only returns leaf tasks — parent tasks act as groupings
that resolve when all their children are done. Nesting supports up to 10 levels.

Bulk JSON format (pipe or --file):
  {
    "title": "Auth System",
    "type": "epic",
    "desc": "Implement full auth stack",
    "children": [
      {"title": "JWT middleware", "type": "task"},
      {"title": "Login endpoint", "type": "task", "desc": "POST /auth/login"},
      {
        "title": "OAuth integration",
        "type": "task",
        "children": [
          {"title": "Google provider", "type": "task"},
          {"title": "GitHub provider", "type": "task"}
        ]
      }
    ]
  }

To express blocker dependencies between items in a bulk create, use "ref" and "blockers":
  {
    "title": "Auth System",
    "type": "epic",
    "children": [
      {"ref": "jwt", "title": "JWT middleware", "type": "task"},
      {"title": "Login endpoint", "type": "task", "blockers": ["jwt"]}
    ]
  }
The "ref" field is a local label (not stored); "blockers" lists refs that must complete first.

Examples:
  hive hc create "Implement auth" --type task --parent hc-abc123
  echo '{"title":"Auth System","type":"epic","children":[...]}' | hive hc create
  hive hc create --file epic.json`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "type", Aliases: []string{"t"}, Usage: "item type (epic, task)", Value: "task", Destination: &flagType},
			&cli.StringFlag{Name: "desc", Aliases: []string{"d"}, Usage: "item description", Destination: &flagDesc},
			&cli.StringFlag{Name: "parent", Aliases: []string{"p"}, Usage: "parent item ID", Destination: &flagParentID},
			bulk.Flag(),
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			repoKey := cmd.detectRepoKey(ctx)
			if repoKey == "" {
				log.Warn().Msg("could not detect repo key; items will not be scoped to a repository")
			}

			if c.NArg() == 0 {
				input, err := bulk.Read()
				if err != nil {
					return fmt.Errorf("read input: %w", err)
				}
				items, err := cmd.app.Honeycomb.CreateBulk(ctx, repoKey, input)
				if err != nil {
					return fmt.Errorf("create bulk: %w", err)
				}
				for _, item := range items {
					if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
						return err
					}
				}
				return nil
			}

			itemType, err := hc.ParseItemType(flagType)
			if err != nil {
				return fmt.Errorf("invalid type %q: valid values are epic, task", flagType)
			}

			if itemType == hc.ItemTypeTask && flagParentID == "" {
				return fmt.Errorf("tasks require a --parent; use --type epic to create a root item")
			}

			item, err := cmd.app.Honeycomb.CreateItem(ctx, repoKey, hc.CreateItemInput{
				Title:    c.Args().First(),
				Desc:     flagDesc,
				Type:     itemType,
				ParentID: flagParentID,
			})
			if err != nil {
				return fmt.Errorf("create item: %w", err)
			}

			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HoneycombCmd) listCmd() *cli.Command {
	var (
		flagStatus  string
		flagSession string
		flagJSON    bool
		flagAll     bool
	)
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List items (defaults to open items)",
		UsageText: "hive hc list [epic-id] [--status <status>] [--all] [--session <id>] [--json]",
		Description: `Lists items as a colored tree. Optional positional arg filters by epic ID.

By default only open items are shown. Use --all to show all statuses,
or --status to filter by a specific status.

With --json, outputs flat JSON lines instead of the tree view.

Examples:
  hive hc list
  hive hc list --all
  hive hc list hc-abc123
  hive hc list --status done
  hive hc list --json hc-abc123`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "status", Usage: "filter by status (open, in_progress, done, cancelled)", Destination: &flagStatus},
			&cli.BoolFlag{Name: "all", Aliases: []string{"a"}, Usage: "show all statuses (overrides default open filter)", Destination: &flagAll},
			&cli.StringFlag{Name: "session", Usage: "filter by session ID", Destination: &flagSession},
			&cli.BoolFlag{Name: "json", Usage: "output as JSON lines", Destination: &flagJSON},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			filter := hc.ListFilter{
				RepoKey:   cmd.detectRepoKey(ctx),
				EpicID:    c.Args().First(),
				SessionID: flagSession,
			}
			switch {
			case flagStatus != "":
				status, err := hc.ParseStatus(flagStatus)
				if err != nil {
					return fmt.Errorf("invalid status %q: valid values are open, in_progress, done, cancelled", flagStatus)
				}
				filter.Status = &status
			case !flagAll:
				open := hc.StatusOpen
				filter.Status = &open
			}

			items, err := cmd.app.Honeycomb.ListItems(ctx, filter)
			if err != nil {
				return fmt.Errorf("list items: %w", err)
			}

			if flagJSON {
				for _, item := range items {
					if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
						return err
					}
				}
				return nil
			}

			renderColoredTree(c.Root().Writer, items)
			return nil
		},
	}
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

// renderMarkdown renders markdown to w using glamour when w is a TTY,
// falling back to plain text otherwise.
func renderMarkdown(w io.Writer, markdown string) {
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if r, err := glamour.NewTermRenderer(glamour.WithAutoStyle()); err == nil {
			if rendered, err := r.Render(markdown); err == nil {
				_, _ = fmt.Fprint(w, rendered)
				return
			}
		}
	}
	_, _ = fmt.Fprint(w, markdown)
}

func (cmd *HoneycombCmd) showCmd() *cli.Command {
	var flagJSON bool
	return &cli.Command{
		Name:      "show",
		Aliases:   []string{"view", "get"},
		Usage:     "Show an item and its comments",
		UsageText: "hive hc show <id> [--json]",
		Description: `Shows an item with its comments in a styled view.

With --json, outputs the item and comments as JSON lines (original behavior).

Examples:
  hive hc show hc-abc123
  hive hc show hc-abc123 --json`,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "output as JSON lines", Destination: &flagJSON},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 1 {
				return fmt.Errorf("item ID required as argument")
			}
			id := c.Args().First()

			item, err := cmd.app.Honeycomb.GetItem(ctx, id)
			if err != nil {
				return fmt.Errorf("get item %q: %w", id, err)
			}

			comments, err := cmd.app.Honeycomb.ListComments(ctx, id)
			if err != nil {
				return fmt.Errorf("list comments for %q: %w", id, err)
			}

			if flagJSON {
				if err := iojson.WriteLine(c.Root().Writer, item); err != nil {
					return err
				}
				for _, comment := range comments {
					if err := iojson.WriteLine(c.Root().Writer, comment); err != nil {
						return err
					}
				}
				return nil
			}

			// Resolve epic title for breadcrumb
			var epicTitle string
			if item.EpicID != "" {
				epic, err := cmd.app.Honeycomb.GetItem(ctx, item.EpicID)
				if err != nil {
					log.Debug().Err(err).Str("epic_id", item.EpicID).Msg("failed to resolve epic title")
				} else {
					epicTitle = epic.Title
				}
			}

			var blockers []hc.Item
			for _, blockerID := range item.BlockerIDs {
				b, err := cmd.app.Honeycomb.GetItem(ctx, blockerID)
				if err != nil {
					log.Debug().Err(err).Str("blocker_id", blockerID).Msg("failed to fetch blocker item")
					continue
				}
				blockers = append(blockers, b)
			}

			renderItem(c.Root().Writer, item, comments, epicTitle, blockers)
			return nil
		},
	}
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

func (cmd *HoneycombCmd) updateCmd() *cli.Command {
	var (
		flagStatus        string
		flagAssign        bool
		flagUnassign      bool
		flagAddBlocker    string
		flagRemoveBlocker string
	)
	return &cli.Command{
		Name:      "update",
		Aliases:   []string{"set", "edit"},
		Usage:     "Update an item's status or session assignment",
		UsageText: "hive hc update <id> [--status <status>] [--assign] [--unassign] [--add-blocker <id>] [--remove-blocker <id>]",
		Description: `Updates an item's status and/or session assignment.

Status values: open, in_progress, done, cancelled

Examples:
  hive hc update hc-abc123 --status done
  hive hc update hc-abc123 --status in_progress --assign
  hive hc update hc-abc123 --unassign
  hive hc update hc-abc123 --add-blocker hc-dep456
  hive hc update hc-abc123 --remove-blocker hc-dep456`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "status", Usage: "new status (open, in_progress, done, cancelled)", Destination: &flagStatus},
			&cli.BoolFlag{Name: "assign", Usage: "assign to current session", Destination: &flagAssign},
			&cli.BoolFlag{Name: "unassign", Usage: "remove session assignment", Destination: &flagUnassign},
			&cli.StringFlag{Name: "add-blocker", Usage: "add an explicit blocker (item ID that must complete first)", Destination: &flagAddBlocker},
			&cli.StringFlag{Name: "remove-blocker", Usage: "remove an explicit blocker by item ID", Destination: &flagRemoveBlocker},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 1 {
				return fmt.Errorf("item ID required as argument")
			}
			id := c.Args().First()

			if flagStatus == "" && !flagAssign && !flagUnassign && flagAddBlocker == "" && flagRemoveBlocker == "" {
				return fmt.Errorf("at least one of --status, --assign, --unassign, --add-blocker, or --remove-blocker is required")
			}

			if flagAddBlocker != "" && flagRemoveBlocker != "" {
				return fmt.Errorf("--add-blocker and --remove-blocker are mutually exclusive")
			}

			// Apply item field updates first so that if they fail, no blocker
			// mutations are committed and the caller sees a clean error.
			var item hc.Item
			hasUpdate := flagStatus != "" || flagAssign || flagUnassign
			if hasUpdate {
				var update hc.ItemUpdate

				if flagStatus != "" {
					status, err := hc.ParseStatus(flagStatus)
					if err != nil {
						return fmt.Errorf("invalid status %q: valid values are open, in_progress, done, cancelled", flagStatus)
					}
					update.Status = &status
				}

				if flagAssign && flagUnassign {
					return fmt.Errorf("--assign and --unassign are mutually exclusive")
				}

				if flagAssign {
					sessionID := cmd.detectSession(ctx)
					if sessionID == "" {
						return fmt.Errorf("could not detect current session; use 'hive session' to verify")
					}
					update.SessionID = &sessionID
				}

				if flagUnassign {
					empty := ""
					update.SessionID = &empty
				}

				var err error
				item, err = cmd.app.Honeycomb.UpdateItem(ctx, id, update)
				if err != nil {
					return fmt.Errorf("update item %q: %w", id, err)
				}
			}

			if flagAddBlocker != "" {
				if err := cmd.app.Honeycomb.AddBlocker(ctx, flagAddBlocker, id); err != nil {
					if errors.Is(err, hc.ErrCyclicDependency) {
						return fmt.Errorf("cannot add blocker: would create a cyclic dependency")
					}
					return fmt.Errorf("add blocker: %w", err)
				}
			}

			if flagRemoveBlocker != "" {
				if err := cmd.app.Honeycomb.RemoveBlocker(ctx, flagRemoveBlocker, id); err != nil {
					return fmt.Errorf("remove blocker: %w", err)
				}
			}

			// Re-fetch after any blocker mutation so blocker_ids reflects the
			// current state; UpdateItem returns the row before edges are written.
			if !hasUpdate || flagAddBlocker != "" || flagRemoveBlocker != "" {
				var err error
				item, err = cmd.app.Honeycomb.GetItem(ctx, id)
				if err != nil {
					return fmt.Errorf("get item %q: %w", id, err)
				}
			}

			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HoneycombCmd) nextCmd() *cli.Command {
	var flagAssign bool
	return &cli.Command{
		Name:      "next",
		Usage:     "Get the next actionable task in an epic",
		UsageText: "hive hc next <epic-id> [--assign]",
		Description: `Returns the next actionable leaf task in the given epic.

Actionable means the task has status open or in_progress and no open/in_progress children.

With --assign, resumes an in_progress task for the current session, or claims the next open task.

Examples:
  hive hc next hc-epic123
  hive hc next hc-epic123 --assign`,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "assign", Usage: "assign item to current session and set in_progress", Destination: &flagAssign},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 1 {
				return fmt.Errorf("epic ID required as argument")
			}

			filter := hc.NextFilter{
				EpicID:  c.Args().First(),
				RepoKey: cmd.detectRepoKey(ctx),
			}

			if flagAssign {
				filter.SessionID = cmd.detectSession(ctx)
				if filter.SessionID == "" {
					return fmt.Errorf("could not detect current session; use 'hive session' to verify")
				}
			}

			item, found, err := cmd.app.Honeycomb.Next(ctx, filter)
			if err != nil {
				return fmt.Errorf("next item: %w", err)
			}
			if !found {
				return fmt.Errorf("no actionable tasks found")
			}

			if flagAssign && item.Status != hc.StatusInProgress {
				sessionID := filter.SessionID
				statusInProgress := hc.StatusInProgress
				updated, err := cmd.app.Honeycomb.UpdateItem(ctx, item.ID, hc.ItemUpdate{
					Status:    &statusInProgress,
					SessionID: &sessionID,
				})
				if err != nil {
					return fmt.Errorf("assign item: %w", err)
				}
				item = updated
			}

			return iojson.WriteLine(c.Root().Writer, item)
		},
	}
}

func (cmd *HoneycombCmd) commentCmd() *cli.Command {
	return &cli.Command{
		Name:      "comment",
		Usage:     "Add a comment to an item",
		UsageText: "hive hc comment <id> <message>",
		Description: `Attaches a comment to an item.

Use for recording progress notes, decisions, or handoff context.

Examples:
  hive hc comment hc-abc123 "Decided to use JWT for auth"
  hive hc comment hc-abc123 "Stopping here — middleware wired, handlers pending"`,
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 2 {
				return fmt.Errorf("item ID and message required as arguments")
			}

			comment, err := cmd.app.Honeycomb.AddComment(ctx, c.Args().Get(0), strings.Join(c.Args().Slice()[1:], " "))
			if err != nil {
				return fmt.Errorf("add comment to %q: %w", c.Args().Get(0), err)
			}

			return iojson.WriteLine(c.Root().Writer, comment)
		},
	}
}

func (cmd *HoneycombCmd) contextCmd() *cli.Command {
	var flagJSON bool
	return &cli.Command{
		Name:      "context",
		Aliases:   []string{"ctx"},
		Usage:     "Show context block for an epic",
		UsageText: "hive hc context <epic-id> [--json]",
		Description: `Assembles and displays the context block for an epic.

The context block contains the epic title, task counts by status, tasks assigned
to the current session (with latest comment), and all open/in-progress tasks.

Without --json, outputs a markdown representation suitable for AI agent consumption.
With --json, outputs a single JSON object.

Examples:
  hive hc context hc-epic123
  hive hc context hc-epic123 --json`,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "output as JSON", Destination: &flagJSON},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			if c.NArg() < 1 {
				return fmt.Errorf("epic ID required as argument")
			}

			epicID := c.Args().First()
			cb, err := cmd.app.Honeycomb.Context(ctx, epicID, cmd.detectSession(ctx))
			if err != nil {
				return fmt.Errorf("get context for epic %q: %w", epicID, err)
			}

			if flagJSON {
				return iojson.WriteWith(c.Root().Writer, c.Root().ErrWriter, cb)
			}

			renderMarkdown(c.Root().Writer, cb.String())
			return nil
		},
	}
}

func (cmd *HoneycombCmd) pruneCmd() *cli.Command {
	var (
		flagOlderThan string
		flagStatuses  []string
		flagDryRun    bool
	)
	return &cli.Command{
		Name:      "prune",
		Usage:     "Remove old completed items",
		UsageText: "hive hc prune [--older-than <duration>] [--status <status>...] [--dry-run]",
		Description: `Removes items older than the specified duration with matching statuses.

Defaults to removing all done and cancelled items.

Examples:
  hive hc prune
  hive hc prune --older-than 24h
  hive hc prune --dry-run`,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "older-than", Usage: "only remove items older than this duration (e.g. 24h, 168h)", Value: "0", Destination: &flagOlderThan},
			&cli.StringSliceFlag{Name: "status", Usage: "statuses to prune (default: done, cancelled)", Destination: &flagStatuses},
			&cli.BoolFlag{Name: "dry-run", Usage: "show what would be pruned without removing", Destination: &flagDryRun},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			olderThan, err := time.ParseDuration(flagOlderThan)
			if err != nil {
				return fmt.Errorf("invalid duration %q: %w", flagOlderThan, err)
			}

			statuses := make([]hc.Status, 0, len(flagStatuses))
			if len(flagStatuses) == 0 {
				statuses = []hc.Status{hc.StatusDone, hc.StatusCancelled}
			} else {
				for _, s := range flagStatuses {
					status, err := hc.ParseStatus(s)
					if err != nil {
						return fmt.Errorf("invalid status %q: valid values are open, in_progress, done, cancelled", s)
					}
					statuses = append(statuses, status)
				}
			}

			count, err := cmd.app.Honeycomb.Prune(ctx, hc.PruneOpts{
				OlderThan: olderThan,
				Statuses:  statuses,
				RepoKey:   cmd.detectRepoKey(ctx),
				DryRun:    flagDryRun,
			})
			if err != nil {
				return fmt.Errorf("prune: %w", err)
			}

			action := "pruned"
			if flagDryRun {
				action = "would prune"
			}

			return iojson.WriteWith(c.Root().Writer, c.Root().ErrWriter, map[string]any{
				"action": action,
				"count":  count,
			})
		},
	}
}
