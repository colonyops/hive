package commands

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/platform/driveapi"
	"github.com/colonyops/hive/pkg/iojson"
)

// shortIDLen is how much of a thread id the text output shows: the end,
// since a UUIDv7 starts with its timestamp.
const shortIDLen = 8

func (cmd *DriveCmd) commentsCmd() *cli.Command {
	return &cli.Command{
		Name:      "comments",
		Usage:     "Read and answer the comment threads on a pushed document",
		ArgsUsage: "<path>",
		Description: `Lists the open and outdated threads on a document pushed to a drive server,
with each thread's quote and every reply. --all adds resolved threads, and
--json prints one thread per line.

A thread is named by its id or a unique end of it, as the list shows it.
Replying and resolving need a token with push scope.

Examples:
  hive drive comments plans/ship.md
  hive drive comments reply plans/ship.md 5d0e77b1 "Changed to 48h."
  hive drive comments resolve plans/ship.md 5d0e77b1`,
		Flags: []cli.Flag{
			cmd.driveServerFlag(),
			&cli.BoolFlag{Name: "json", Usage: "output one JSON thread per line", Destination: &cmd.json},
			&cli.BoolFlag{Name: "all", Usage: "include resolved threads", Destination: &cmd.all},
		},
		Action: cmd.runComments,
		Commands: []*cli.Command{
			{
				Name:      "reply",
				Usage:     "Reply to a thread",
				ArgsUsage: "<path> <thread> <text>",
				Flags:     []cli.Flag{cmd.driveServerFlag()},
				Action:    cmd.runCommentsReply,
			},
			{
				Name:      "resolve",
				Usage:     "Resolve a thread",
				ArgsUsage: "<path> <thread>",
				Flags:     []cli.Flag{cmd.driveServerFlag()},
				Action:    func(ctx context.Context, c *cli.Command) error { return cmd.runCommentsResolve(ctx, c, true) },
			},
			{
				Name:      "reopen",
				Usage:     "Reopen a resolved thread",
				ArgsUsage: "<path> <thread>",
				Flags:     []cli.Flag{cmd.driveServerFlag()},
				Action:    func(ctx context.Context, c *cli.Command) error { return cmd.runCommentsResolve(ctx, c, false) },
			},
		},
	}
}

func (cmd *DriveCmd) runComments(ctx context.Context, c *cli.Command) error {
	if c.Args().Len() != 1 {
		return invalidf("comments: exactly one path is required")
	}
	path := c.Args().First()
	remote, space, server, err := cmd.remoteTarget(ctx)
	if err != nil {
		return err
	}
	list, err := remote.Threads(ctx, server, space, path)
	if err != nil {
		return err
	}
	shown := make([]driveapi.Thread, 0, len(list.Threads))
	for _, t := range list.Threads {
		if cmd.all || t.ResolvedAt == nil {
			shown = append(shown, t)
		}
	}
	if cmd.json {
		for _, t := range shown {
			if err := iojson.WriteLine(c.Root().Writer, t); err != nil {
				return err
			}
		}
		return nil
	}

	var body []byte
	if hasAnchor(shown) {
		// The context lines are a convenience; the threads print without them.
		if body, err = remote.Read(ctx, server, space, path); err != nil {
			_, _ = fmt.Fprintf(c.Root().ErrWriter, "could not read the document for context: %v\n", err)
		}
	}
	writeThreads(c.Root().Writer, path, list.Threads, shown, body, time.Now())
	return nil
}

func (cmd *DriveCmd) runCommentsReply(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) != 3 || strings.TrimSpace(args[2]) == "" {
		return invalidf("comments reply: <path> <thread> <text> are required")
	}
	remote, space, server, err := cmd.remoteTarget(ctx)
	if err != nil {
		return err
	}
	comment, err := remote.Reply(ctx, server, space, args[0], args[1], args[2])
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.Root().Writer, "replied to #%s\n", shortID(comment.ThreadID))
	return nil
}

func (cmd *DriveCmd) runCommentsResolve(ctx context.Context, c *cli.Command, resolved bool) error {
	verb, done := "reopen", "reopened"
	if resolved {
		verb, done = "resolve", "resolved"
	}
	args := c.Args().Slice()
	if len(args) != 2 {
		return invalidf("comments %s: <path> <thread> are required", verb)
	}
	remote, space, server, err := cmd.remoteTarget(ctx)
	if err != nil {
		return err
	}
	t, err := remote.SetResolved(ctx, server, space, args[0], args[1], resolved)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.Root().Writer, "%s #%s\n", done, shortID(t.ID))
	return nil
}

func hasAnchor(threads []driveapi.Thread) bool {
	for _, t := range threads {
		if t.Anchor != nil {
			return true
		}
	}
	return false
}

// writeThreads prints the design's text layout: a count line, then each
// shown thread with its quote, the source line around it, and its comments.
func writeThreads(w io.Writer, path string, all, shown []driveapi.Thread, body []byte, now time.Time) {
	var open, outdated, resolved int
	for _, t := range all {
		switch {
		case t.ResolvedAt != nil:
			resolved++
		case t.Outdated:
			outdated++
		default:
			open++
		}
	}
	_, _ = fmt.Fprintf(w, "%s  %d open, %d outdated, %d resolved\n", path, open, outdated, resolved)

	lines := strings.Split(string(body), "\n")
	for _, t := range shown {
		state := "open"
		switch {
		case t.ResolvedAt != nil:
			state = "resolved"
		case t.Outdated:
			state = "outdated"
		}
		where := "general"
		if a := t.Anchor; a != nil {
			where = fmt.Sprintf("L%d-%d", a.StartLine, a.EndLine)
			if a.StartLine == a.EndLine {
				where = fmt.Sprintf("L%d", a.StartLine)
			}
		}
		_, _ = fmt.Fprintf(w, "\n#%s  %s  %s\n", shortID(t.ID), where, state)
		if a := t.Anchor; a != nil {
			_, _ = fmt.Fprintf(w, "  > %q\n", clip(oneLine(a.Quote), 120))
			if !t.Outdated && body != nil && a.StartLine >= 1 && a.StartLine <= len(lines) {
				_, _ = fmt.Fprintf(w, "  > in: %q\n", clip(strings.TrimSpace(lines[a.StartLine-1]), 120))
			}
		}
		for i, cm := range t.Comments {
			indent := "  "
			if i > 0 {
				indent = "    "
			}
			if cm.Deleted {
				_, _ = fmt.Fprintf(w, "%s(comment deleted)\n", indent)
				continue
			}
			_, _ = fmt.Fprintf(w, "%s%s %s: %s\n", indent, authorLabel(cm.Author), age(now, cm.CreatedAt),
				strings.ReplaceAll(cm.Body, "\n", "\n"+indent+"  "))
		}
	}
}

func authorLabel(a driveapi.Author) string {
	label := a.Name
	if a.Email != "" {
		label += " (" + a.Email + ")"
	}
	if a.Agent != "" {
		label += " [agent: " + a.Agent + "]"
	}
	return label
}

func age(now, at time.Time) string {
	d := now.Sub(at)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func shortID(id string) string {
	if len(id) <= shortIDLen {
		return id
	}
	return id[len(id)-shortIDLen:]
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
