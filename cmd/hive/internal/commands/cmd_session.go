package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/colonyops/hive/cmd/hive/internal/app"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/sessionapi"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/urfave/cli/v3"
)

type SessionCmd struct {
	flags *Flags
	app   *app.App

	// per-subcommand flags
	infoJSON bool
	lsJSON   bool
	lsTags   []string

	showJSON bool

	createJSON  bool
	createFlags createSessionFlags

	updateJSON       bool
	updateName       string
	updateGroup      string
	updateClearGroup bool
	updateAddTags    []string
	updateRemoveTags []string
	updateClearTags  bool

	deleteJSON  bool
	deleteForce bool

	recycleJSON  bool
	recycleForce bool
}

// NewSessionCmd creates a new session command
func NewSessionCmd(flags *Flags, app *app.App) *SessionCmd {
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
				Usage:       "filter sessions by tag (repeatable, all tags must match, case-insensitive)",
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
	Server        string    `json:"server"`
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

func buildSessionJSON(server string, s session.Session) sessionJSON {
	return sessionJSON{
		Server:        server,
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
		Tags:          nonNilTags(s.Tags),
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func printSessionHuman(out io.Writer, server string, sess session.Session) {
	if server != session.LocalServer {
		_, _ = fmt.Fprintf(out, "Server:      %s\n", server)
	}
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
	sessionID, err := cmd.app.Sessions().DetectSession(ctx)
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
	sess, err := cmd.app.SessionAPI().GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	out := c.Root().Writer

	if cmd.infoJSON {
		return iojson.WriteLine(out, buildSessionJSON(cmd.app.DefaultServer(), sess))
	}

	printSessionHuman(out, cmd.app.DefaultServer(), sess)
	return nil
}

func (cmd *SessionCmd) showCmd() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show details for a session by ID",
		UsageText: "hive session show <[server:]id> [--json]",
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
	ref, api, err := cmd.resolve(c.Args().First())
	if err != nil {
		return err
	}

	sess, err := api.GetSession(ctx, ref.ID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	out := c.Root().Writer
	if cmd.showJSON {
		return iojson.WriteLine(out, buildSessionJSON(ref.Server, sess))
	}

	printSessionHuman(out, ref.Server, sess)
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
		Flags: append(sessionCreateFlags(&cmd.createFlags),
			&cli.BoolFlag{
				Name:        "json",
				Usage:       "write the created session as JSON to stdout",
				Destination: &cmd.createJSON,
			},
		),
		Action: cmd.runCreate,
	}
}

func (cmd *SessionCmd) runCreate(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) == 0 {
		return fmt.Errorf("session name required\n\nUsage: hive session create <name...>")
	}
	name := strings.Join(args, " ")

	// Keep stdout clean for --json output; progress goes to stderr.
	sess, err := createSessionFromFlags(ctx, cmd.app, name, &cmd.createFlags, os.Stderr)
	if err != nil {
		return err
	}

	if cmd.createJSON {
		return iojson.WriteLine(c.Root().Writer, buildSessionJSON(cmd.app.DefaultServer(), *sess))
	}

	fmt.Fprintf(os.Stderr, "Session created\n  %s\n", sess.Path)
	return nil
}

func (cmd *SessionCmd) updateCmd() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update a session's name, group, or tags",
		UsageText: "hive session update <[server:]id> [--name <name>] [--group <group> | --clear-group] [--add-tag <tag>]... [--remove-tag <tag>]... [--clear-tags] [--json]",
		Description: `Updates mutable fields on an existing session.

At least one flag that changes the session is required. Tag flags apply in
order: --clear-tags, then --add-tag, then --remove-tag. Tags compare
case-insensitively.

Examples:
  hive session update abc123 --name "New Name"
  hive session update abc123 --group backend
  hive session update abc123 --clear-group
  hive session update abc123 --add-tag auto --add-tag backend
  hive session update abc123 --remove-tag auto --add-tag orchestrated`,
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
			&cli.StringSliceFlag{
				Name:        "add-tag",
				Usage:       "add a tag (repeatable)",
				Destination: &cmd.updateAddTags,
			},
			&cli.StringSliceFlag{
				Name:        "remove-tag",
				Usage:       "remove a tag (repeatable)",
				Destination: &cmd.updateRemoveTags,
			},
			&cli.BoolFlag{
				Name:        "clear-tags",
				Usage:       "remove every tag",
				Destination: &cmd.updateClearTags,
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
	ref, api, err := cmd.resolve(c.Args().First())
	if err != nil {
		return err
	}

	if cmd.updateGroup != "" && cmd.updateClearGroup {
		return fmt.Errorf("--group and --clear-group are mutually exclusive")
	}
	editsTags := len(cmd.updateAddTags) > 0 || len(cmd.updateRemoveTags) > 0 || cmd.updateClearTags
	if cmd.updateName == "" && cmd.updateGroup == "" && !cmd.updateClearGroup && !editsTags {
		return fmt.Errorf("nothing to update: provide --name, --group, --clear-group, or a tag flag")
	}
	// Checked before any write, so a bad tag leaves the session untouched.
	addTags, err := session.NormalizeTags(cmd.updateAddTags)
	if err != nil {
		return err
	}

	if cmd.updateName != "" {
		if err := api.RenameSession(ctx, ref.ID, cmd.updateName); err != nil {
			return fmt.Errorf("rename session: %w", err)
		}
	}

	if cmd.updateGroup != "" || cmd.updateClearGroup {
		group := cmd.updateGroup
		if cmd.updateClearGroup {
			group = ""
		}
		if err := api.SetSessionGroup(ctx, ref.ID, group); err != nil {
			return fmt.Errorf("set session group: %w", err)
		}
	}

	switch {
	case cmd.updateClearTags:
		tags := slices.DeleteFunc(addTags, func(tag string) bool {
			return slices.ContainsFunc(cmd.updateRemoveTags, func(remove string) bool {
				return strings.EqualFold(strings.TrimSpace(remove), tag)
			})
		})
		if _, err := api.SetSessionTags(ctx, ref.ID, tags); err != nil {
			return fmt.Errorf("set session tags: %w", err)
		}
	case len(addTags) > 0 || len(cmd.updateRemoveTags) > 0:
		if _, err := api.EditSessionTags(ctx, ref.ID, addTags, cmd.updateRemoveTags); err != nil {
			return fmt.Errorf("edit session tags: %w", err)
		}
	}

	sess, err := api.GetSession(ctx, ref.ID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	if cmd.updateJSON {
		return iojson.WriteLine(c.Root().Writer, buildSessionJSON(ref.Server, sess))
	}

	fmt.Fprintf(os.Stderr, "Session %s updated\n", ref)
	return nil
}

// lsSessionInfo is the JSON output format for hive session ls --json.
type lsSessionInfo struct {
	Server string   `json:"server"`
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Repo   string   `json:"repo"`
	Inbox  string   `json:"inbox"`
	State  string   `json:"state"`
	Unread int      `json:"unread"`
	Tags   []string `json:"tags"`
}

func (cmd *SessionCmd) runLs(ctx context.Context, c *cli.Command) error {
	sessions, err := cmd.app.SessionAPI().ListSessions(ctx)
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
		if !s.HasAllTags(cmd.lsTags) {
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
		UsageText: "hive session delete <[server:]id>",
		Description: `Permanently removes a session, its cloned directory, and any associated tmux session.

If the session has uncommitted changes or unpushed commits, the delete is
refused unless --force is passed. For worktree sessions the check can report
false positives: the shared bare clone has no remote-tracking refs, so a
feature branch that was pushed but not merged still counts as unpushed. Use
--force after verifying the push.

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
	ref, api, err := cmd.resolve(c.Args().First())
	if err != nil {
		return err
	}

	if !cmd.deleteForce {
		if err := cmd.checkRisk(ctx, api, ref, "delete"); err != nil {
			return err
		}
	}

	if err := api.DeleteSession(ctx, ref.ID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	if cmd.deleteJSON {
		return iojson.WriteLine(c.Root().Writer, map[string]any{"server": ref.Server, "id": ref.ID, "deleted": true})
	}

	fmt.Fprintf(os.Stderr, "Session %s deleted\n", ref)
	return nil
}

func (cmd *SessionCmd) recycleCmd() *cli.Command {
	return &cli.Command{
		Name:      "recycle",
		Usage:     "Recycle a session back to the pool",
		UsageText: "hive session recycle <[server:]id>",
		Description: `Recycles a full-clone session so its checkout can be reused for a new task.

For worktree sessions, removes the checkout and session record while retaining the shared bare clone. This keeps future worktree creation fast without retaining stale session state.

If the session has uncommitted changes or unpushed commits, the recycle is
refused unless --force is passed. For worktree sessions the check can report
false positives: the shared bare clone has no remote-tracking refs, so a
feature branch that was pushed but not merged still counts as unpushed. Use
--force after verifying the push.`,
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
	ref, api, err := cmd.resolve(c.Args().First())
	if err != nil {
		return err
	}

	if !cmd.recycleForce {
		if err := cmd.checkRisk(ctx, api, ref, "recycle"); err != nil {
			return err
		}
	}

	if err := api.RecycleSession(ctx, ref.ID, os.Stderr); err != nil {
		return fmt.Errorf("recycle session: %w", err)
	}

	if cmd.recycleJSON {
		// Worktree sessions are deleted on recycle, so the record may be gone.
		sess, err := api.GetSession(ctx, ref.ID)
		switch {
		case errors.Is(err, session.ErrNotFound):
			return iojson.WriteLine(c.Root().Writer, map[string]any{"server": ref.Server, "id": ref.ID, "deleted": true})
		case err != nil:
			return fmt.Errorf("get session after recycle: %w", err)
		}
		return iojson.WriteLine(c.Root().Writer, buildSessionJSON(ref.Server, sess))
	}

	fmt.Fprintf(os.Stderr, "Session %s recycled\n", ref)
	return nil
}

func (cmd *SessionCmd) buildLsSessionInfo(ctx context.Context, s session.Session) lsSessionInfo {
	info := lsSessionInfo{
		Server: cmd.app.DefaultServer(),
		ID:     s.ID,
		Name:   s.Name,
		Repo:   git.ExtractRepoName(s.Remote),
		Inbox:  s.InboxTopic(),
		State:  string(s.State),
		Unread: 0,
		Tags:   nonNilTags(s.Tags),
	}

	// Count unread inbox messages
	if msgs, err := cmd.app.Messages().GetUnread(ctx, s.ID, s.InboxTopic()); err == nil {
		info.Unread = len(msgs)
	}

	return info
}

// checkRisk returns an error describing uncommitted or unpushed work that
// would be lost if the session were destroyed by the given action.
func (cmd *SessionCmd) checkRisk(ctx context.Context, api sessionapi.API, ref session.Ref, action string) error {
	risk, err := api.CheckSessionRisk(ctx, ref.ID)
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
	return fmt.Errorf("session %s has %s; use --force to %s anyway", ref, strings.Join(reasons, " and "), action)
}

// resolve reads a session argument, "<server>:<id>" or a bare id on the
// default server, and finds that server's session API.
func (cmd *SessionCmd) resolve(arg string) (session.Ref, sessionapi.API, error) {
	if arg == "" {
		return session.Ref{}, nil, fmt.Errorf("session ID required")
	}
	ref, err := session.ParseRef(arg, cmd.app.DefaultServer())
	if err != nil {
		return session.Ref{}, nil, err
	}
	api, err := cmd.app.ServerAPI(ref.Server)
	if err != nil {
		return session.Ref{}, nil, err
	}
	return ref, api, nil
}

// nonNilTags keeps JSON output an array, never null.
func nonNilTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}
