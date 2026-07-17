package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type SessionCmd struct {
	flags *Flags
	app   *hive.App

	// per-subcommand flags
	infoJSON bool
	lsJSON   bool
	lsTags   []string

	showJSON bool

	createJSON          bool
	createRemote        string
	createSource        string
	createBackground    bool
	createCloneStrategy string
	createAgent         string
	createTags          []string

	updateJSON       bool
	updateName       string
	updateGroup      string
	updateClearGroup bool

	deleteJSON  bool
	deleteForce bool

	recycleJSON  bool
	recycleForce bool
}

// NewSessionCmd creates a new session command
func NewSessionCmd(flags *Flags, app *hive.App) *SessionCmd {
	return &SessionCmd{flags: flags, app: app}
}

// Register adds the session command group and a top-level "ls" alias.
func (cmd *SessionCmd) Register(app *cli.Command) *cli.Command {
	lsCommand := cmd.lsCmd()

	app.Commands = append(app.Commands,
		&cli.Command{
			Name:  "session",
			Usage: "Session management commands",
			Description: `Commands for managing and inspecting hive sessions.

Use 'hive session list' to list all sessions.
Use 'hive session info' to get details about the current session.`,
			Commands: []*cli.Command{
				lsCommand,
				cmd.infoCmd(),
				cmd.showCmd(),
				cmd.createCmd(),
				cmd.updateCmd(),
				cmd.deleteCmd(),
				cmd.recycleCmd(),
			},
		},
		// Top-level alias: "hive ls" -> "hive session list"
		&cli.Command{
			Name:      "ls",
			Usage:     "List all sessions (alias for 'session list')",
			UsageText: "hive ls [--json]",
			Hidden:    true,
			Flags:     lsCommand.Flags,
			Action:    lsCommand.Action,
		},
	)

	return app
}

func (cmd *SessionCmd) lsCmd() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Aliases:   []string{"ls"},
		Usage:     "List all sessions",
		UsageText: "hive session list [--json]",
		Description: `Displays a table of all sessions with their repo, name, state, and path.

Use --json for LLM-friendly output with additional fields like inbox topic and unread count.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON lines with inbox info",
				Destination: &cmd.lsJSON,
			},
			&cli.StringSliceFlag{
				Name:        "tags",
				Aliases:     []string{"t"},
				Usage:       "filter sessions by tag (repeatable, all tags must match)",
				Destination: &cmd.lsTags,
			},
		},
		Action: cmd.runLs,
	}
}

func (cmd *SessionCmd) infoCmd() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: "Show current session information",
		Description: `Displays information about the current hive session based on the working directory.

This command is useful for LLMs to discover their session ID and inbox topic.

Example output (--json):
  {"id":"abc123","name":"Fix Auth Bug","inbox":"agent.abc123.inbox",...}`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON (recommended for LLMs)",
				Destination: &cmd.infoJSON,
			},
		},
		Action: cmd.runInfo,
	}
}

// sessionJSON is the machine-readable representation of a session used by
// session info/show/create/update/recycle --json output.
type sessionJSON struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	Repo          string    `json:"repo"`
	Remote        string    `json:"remote"`
	Path          string    `json:"path"`
	Inbox         string    `json:"inbox"`
	State         string    `json:"state"`
	Group         string    `json:"group,omitempty"`
	CloneStrategy string    `json:"clone_strategy,omitempty"`
	Tags          []string  `json:"tags"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func buildSessionJSON(s session.Session) sessionJSON {
	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}
	return sessionJSON{
		ID:            s.ID,
		Name:          s.Name,
		Slug:          s.Slug,
		Repo:          git.ExtractRepoName(s.Remote),
		Remote:        s.Remote,
		Path:          s.Path,
		Inbox:         s.InboxTopic(),
		State:         string(s.State),
		Group:         s.GetMeta(session.MetaGroup),
		CloneStrategy: s.CloneStrategy,
		Tags:          tags,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func printSessionHuman(out io.Writer, sess session.Session) {
	_, _ = fmt.Fprintf(out, "Session ID:  %s\n", sess.ID)
	_, _ = fmt.Fprintf(out, "Name:        %s\n", sess.Name)
	_, _ = fmt.Fprintf(out, "Repository:  %s\n", git.ExtractRepoName(sess.Remote))
	_, _ = fmt.Fprintf(out, "Inbox:       %s\n", sess.InboxTopic())
	_, _ = fmt.Fprintf(out, "Path:        %s\n", sess.Path)
	_, _ = fmt.Fprintf(out, "State:       %s\n", sess.State)
	if group := sess.GetMeta(session.MetaGroup); group != "" {
		_, _ = fmt.Fprintf(out, "Group:       %s\n", group)
	}
	if len(sess.Tags) > 0 {
		_, _ = fmt.Fprintf(out, "Tags:        %s\n", strings.Join(sess.Tags, ", "))
	}
}

func (cmd *SessionCmd) runInfo(ctx context.Context, c *cli.Command) error {
	// Detect session from current working directory
	sessionID, err := cmd.app.Sessions.DetectSession(ctx)
	if err != nil {
		return fmt.Errorf("detect session: %w", err)
	}

	if sessionID == "" {
		if cmd.infoJSON {
			_, _ = fmt.Fprintln(c.Root().Writer, "{\"error\":\"not in a hive session\"}")
			return nil
		}
		fmt.Fprintf(os.Stderr, "Not in a hive session\nRun this command from within a hive session directory\n")
		return nil
	}

	// Get full session details
	sess, err := cmd.app.Sessions.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	out := c.Root().Writer

	if cmd.infoJSON {
		return iojson.WriteLine(out, buildSessionJSON(sess))
	}

	printSessionHuman(out, sess)
	return nil
}

func (cmd *SessionCmd) showCmd() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show details for a session by ID",
		UsageText: "hive session show <id> [--json]",
		Description: `Displays details for a specific session.

Unlike 'hive session info', which detects the session from the current
working directory, this command looks up a session by its ID.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "output as JSON (recommended for LLMs)",
				Destination: &cmd.showJSON,
			},
		},
		Action: cmd.runShow,
	}
}

func (cmd *SessionCmd) runShow(ctx context.Context, c *cli.Command) error {
	id := c.Args().First()
	if id == "" {
		return fmt.Errorf("session ID required")
	}

	sess, err := cmd.app.Sessions.GetSession(ctx, id)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	out := c.Root().Writer
	if cmd.showJSON {
		return iojson.WriteLine(out, buildSessionJSON(sess))
	}

	printSessionHuman(out, sess)
	return nil
}

func (cmd *SessionCmd) createCmd() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create a new agent session",
		UsageText: "hive session create <name...> [--json]",
		Description: `Creates a new isolated git environment for an AI agent session.

Equivalent to 'hive new', but designed for machine consumption: with --json
the created session record (including its ID and inbox topic) is written to
stdout while progress output goes to stderr.

Example:
  hive session create --json --background --remote <url> worker-1`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "remote",
				Aliases:     []string{"r"},
				Usage:       "git remote URL (defaults to current directory's origin)",
				Destination: &cmd.createRemote,
			},
			&cli.StringFlag{
				Name:        "source",
				Aliases:     []string{"s"},
				Usage:       "source directory for file copying (defaults to current directory)",
				Destination: &cmd.createSource,
			},
			&cli.BoolFlag{
				Name:        "background",
				Aliases:     []string{"bg"},
				Usage:       "create session without attaching to tmux",
				Destination: &cmd.createBackground,
			},
			&cli.StringFlag{
				Name:        "clone-strategy",
				Usage:       "clone strategy: full or worktree",
				Destination: &cmd.createCloneStrategy,
			},
			&cli.StringFlag{
				Name:        "agent",
				Aliases:     []string{"a"},
				Usage:       "agent profile key from agents config",
				Destination: &cmd.createAgent,
			},
			&cli.StringSliceFlag{
				Name:        "tags",
				Aliases:     []string{"t"},
				Usage:       "tags to attach to the session (repeatable)",
				Destination: &cmd.createTags,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "write the created session as JSON to stdout",
				Destination: &cmd.createJSON,
			},
		},
		Action: cmd.runCreate,
	}
}

func (cmd *SessionCmd) runCreate(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("session name required\n\nUsage: hive session create <name...>")
	}
	name := strings.Join(args, " ")

	if cmd.createAgent != "" {
		if _, ok := cmd.app.Config.Agents.Profiles[cmd.createAgent]; !ok {
			return fmt.Errorf("unknown agent %q", cmd.createAgent)
		}
	}

	source := cmd.createSource
	if source == "" {
		var err error
		source, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("determine source directory: %w", err)
		}
	}

	opts := hive.CreateOptions{
		Name:          name,
		Remote:        cmd.createRemote,
		Source:        source,
		Background:    cmd.createBackground,
		CloneStrategy: cmd.createCloneStrategy,
		AgentKey:      cmd.createAgent,
		Tags:          cmd.createTags,
		// Keep stdout clean for --json output; progress goes to stderr.
		Progress: os.Stderr,
	}

	sess, err := cmd.app.Sessions.CreateSession(ctx, opts)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	if cmd.createJSON {
		return iojson.WriteLine(c.Root().Writer, buildSessionJSON(*sess))
	}

	fmt.Fprintf(os.Stderr, "Session created\n  %s\n", sess.Path)
	return nil
}

func (cmd *SessionCmd) updateCmd() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a session's name or group",
		UsageText: "hive session update <id> [--name <name>] [--group <group> | --clear-group] [--json]",
		Description: `Updates mutable fields on an existing session.

At least one of --name, --group, or --clear-group is required.

Examples:
  hive session update abc123 --name "New Name"
  hive session update abc123 --group backend
  hive session update abc123 --clear-group`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "name",
				Usage:       "new session name (also updates slug)",
				Destination: &cmd.updateName,
			},
			&cli.StringFlag{
				Name:        "group",
				Usage:       "assign the session to a group",
				Destination: &cmd.updateGroup,
			},
			&cli.BoolFlag{
				Name:        "clear-group",
				Usage:       "clear the session's group assignment",
				Destination: &cmd.updateClearGroup,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "write the updated session as JSON to stdout",
				Destination: &cmd.updateJSON,
			},
		},
		Action: cmd.runUpdate,
	}
}

func (cmd *SessionCmd) runUpdate(ctx context.Context, c *cli.Command) error {
	id := c.Args().First()
	if id == "" {
		return fmt.Errorf("session ID required")
	}

	if cmd.updateGroup != "" && cmd.updateClearGroup {
		return fmt.Errorf("--group and --clear-group are mutually exclusive")
	}
	if cmd.updateName == "" && cmd.updateGroup == "" && !cmd.updateClearGroup {
		return fmt.Errorf("nothing to update: provide --name, --group, or --clear-group")
	}

	if cmd.updateName != "" {
		if err := cmd.app.Sessions.RenameSession(ctx, id, cmd.updateName); err != nil {
			return fmt.Errorf("rename session: %w", err)
		}
	}

	if cmd.updateGroup != "" || cmd.updateClearGroup {
		group := cmd.updateGroup
		if cmd.updateClearGroup {
			group = ""
		}
		if err := cmd.app.Sessions.SetSessionGroup(ctx, id, group); err != nil {
			return fmt.Errorf("set session group: %w", err)
		}
	}

	sess, err := cmd.app.Sessions.GetSession(ctx, id)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	if cmd.updateJSON {
		return iojson.WriteLine(c.Root().Writer, buildSessionJSON(sess))
	}

	fmt.Fprintf(os.Stderr, "Session %s updated\n", id)
	return nil
}

// lsSessionInfo is the JSON output format for hive session ls --json.
type lsSessionInfo struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Repo   string   `json:"repo"`
	Inbox  string   `json:"inbox"`
	State  string   `json:"state"`
	Unread int      `json:"unread"`
	Tags   []string `json:"tags"`
}

func (cmd *SessionCmd) runLs(ctx context.Context, c *cli.Command) error {
	sessions, err := cmd.app.Sessions.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		if !cmd.lsJSON {
			fmt.Fprintf(os.Stderr, "No sessions found\n")
		}
		return nil
	}

	// Separate normal and corrupted sessions, applying tag filter if specified
	var normal, corrupted []session.Session
	for _, s := range sessions {
		if s.State == session.StateCorrupted {
			corrupted = append(corrupted, s)
			continue
		}
		if len(cmd.lsTags) > 0 && !sessionHasAllTags(s, cmd.lsTags) {
			continue
		}
		normal = append(normal, s)
	}

	// Sort by repository name
	slices.SortFunc(normal, func(a, b session.Session) int {
		return strings.Compare(git.ExtractRepoName(a.Remote), git.ExtractRepoName(b.Remote))
	})

	out := c.Root().Writer

	// JSON output mode
	if cmd.lsJSON {
		for _, s := range normal {
			info := cmd.buildLsSessionInfo(ctx, s)
			if err := iojson.WriteLine(out, info); err != nil {
				return fmt.Errorf("encode session: %w", err)
			}
		}
		return nil
	}

	// Table output mode
	if len(normal) > 0 {
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "REPO\tNAME\tSTATE\tPATH")

		for _, s := range normal {
			repo := git.ExtractRepoName(s.Remote)
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", repo, s.Name, s.State, s.Path)
		}

		_ = w.Flush()
	}

	if len(corrupted) > 0 {
		_, _ = fmt.Fprintln(out)
		fmt.Fprintf(os.Stderr, "Found %d corrupted session(s) with invalid git repositories:\n", len(corrupted))
		for _, s := range corrupted {
			repo := git.ExtractRepoName(s.Remote)
			fmt.Fprintf(os.Stderr, "  %s (%s)\n", repo, s.Path)
		}
		_, _ = fmt.Fprintln(out)
		fmt.Fprintf(os.Stderr, "Run 'hive prune' to clean up\n")
	}

	return nil
}

func (cmd *SessionCmd) deleteCmd() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Delete a session and its directory",
		UsageText: "hive session delete <id>",
		Description: `Permanently removes a session, its cloned directory, and any associated tmux session.

If the session has uncommitted changes or unpushed commits, the delete is
refused unless --force is passed.

This action cannot be undone. Use 'hive session recycle' to preserve the directory for reuse.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "force",
				Aliases:     []string{"f"},
				Usage:       "delete even if the session has uncommitted or unpushed work",
				Destination: &cmd.deleteForce,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "write the delete result as JSON to stdout",
				Destination: &cmd.deleteJSON,
			},
		},
		Action: cmd.runDelete,
	}
}

func (cmd *SessionCmd) runDelete(ctx context.Context, c *cli.Command) error {
	id := c.Args().First()
	if id == "" {
		return fmt.Errorf("session ID required")
	}

	if !cmd.deleteForce {
		if err := cmd.checkRisk(ctx, id, "delete"); err != nil {
			return err
		}
	}

	if err := cmd.app.Sessions.DeleteSession(ctx, id); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	if cmd.deleteJSON {
		return iojson.WriteLine(c.Root().Writer, map[string]any{"id": id, "deleted": true})
	}

	fmt.Fprintf(os.Stderr, "Session %s deleted\n", id)
	return nil
}

func (cmd *SessionCmd) recycleCmd() *cli.Command {
	return &cli.Command{
		Name:      "recycle",
		Usage:     "Recycle a session back to the pool",
		UsageText: "hive session recycle <id>",
		Description: `Recycles a full-clone session so its checkout can be reused for a new task.

For worktree sessions, removes the checkout and session record while retaining the shared bare clone. This keeps future worktree creation fast without retaining stale session state.

If the session has uncommitted changes or unpushed commits, the recycle is
refused unless --force is passed.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "force",
				Aliases:     []string{"f"},
				Usage:       "recycle even if the session has uncommitted or unpushed work",
				Destination: &cmd.recycleForce,
			},
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "write the recycle result as JSON to stdout",
				Destination: &cmd.recycleJSON,
			},
		},
		Action: cmd.runRecycle,
	}
}

func (cmd *SessionCmd) runRecycle(ctx context.Context, c *cli.Command) error {
	id := c.Args().First()
	if id == "" {
		return fmt.Errorf("session ID required")
	}

	if !cmd.recycleForce {
		if err := cmd.checkRisk(ctx, id, "recycle"); err != nil {
			return err
		}
	}

	if err := cmd.app.Sessions.RecycleSession(ctx, id, os.Stderr); err != nil {
		return fmt.Errorf("recycle session: %w", err)
	}

	if cmd.recycleJSON {
		// Worktree sessions are deleted on recycle, so the record may be gone.
		sess, err := cmd.app.Sessions.GetSession(ctx, id)
		if err != nil {
			return iojson.WriteLine(c.Root().Writer, map[string]any{"id": id, "deleted": true})
		}
		return iojson.WriteLine(c.Root().Writer, buildSessionJSON(sess))
	}

	fmt.Fprintf(os.Stderr, "Session %s recycled\n", id)
	return nil
}

func (cmd *SessionCmd) buildLsSessionInfo(ctx context.Context, s session.Session) lsSessionInfo {
	tags := s.Tags
	if tags == nil {
		tags = []string{}
	}
	info := lsSessionInfo{
		ID:     s.ID,
		Name:   s.Name,
		Repo:   git.ExtractRepoName(s.Remote),
		Inbox:  s.InboxTopic(),
		State:  string(s.State),
		Unread: 0,
		Tags:   tags,
	}

	// Count unread inbox messages
	if msgs, err := cmd.app.Messages.GetUnread(ctx, s.ID, s.InboxTopic()); err == nil {
		info.Unread = len(msgs)
	}

	return info
}

// checkRisk returns an error describing uncommitted or unpushed work that
// would be lost if the session were destroyed by the given action.
func (cmd *SessionCmd) checkRisk(ctx context.Context, id, action string) error {
	risk, err := cmd.app.Sessions.CheckSessionRisk(ctx, id)
	if err != nil {
		return fmt.Errorf("check session risk: %w", err)
	}
	if !risk.HasRisk() {
		return nil
	}
	var reasons []string
	if risk.UncommittedChanges {
		reasons = append(reasons, "uncommitted changes")
	}
	if risk.UnpushedCommits {
		reasons = append(reasons, "unpushed commits")
	}
	return fmt.Errorf("session %s has %s; use --force to %s anyway", id, strings.Join(reasons, " and "), action)
}

// sessionHasAllTags returns true if the session has every tag in required.
func sessionHasAllTags(s session.Session, required []string) bool {
	tagSet := make(map[string]struct{}, len(s.Tags))
	for _, t := range s.Tags {
		tagSet[t] = struct{}{}
	}
	for _, r := range required {
		if _, ok := tagSet[r]; !ok {
			return false
		}
	}
	return true
}
