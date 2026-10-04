package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive/prompt"
	"github.com/colonyops/hive/pkg/iojson"
)

type sessionDriveFlags struct {
	noSubmit bool
	lines    int
	json     bool
}

func (cmd *SessionCmd) driveFlags(withSubmit bool) []cli.Flag {
	flags := []cli.Flag{
		&cli.IntFlag{Name: "lines", Usage: "screen lines to show", Value: 20, Destination: &cmd.drive.lines},
		&cli.BoolFlag{Name: "json", Usage: "output as JSON (recommended for LLMs)", Destination: &cmd.drive.json},
	}
	if withSubmit {
		flags = append(flags, &cli.BoolFlag{Name: "no-submit", Usage: "type the text without pressing Enter", Destination: &cmd.drive.noSubmit})
	}
	return flags
}

func (cmd *SessionCmd) sendCmd() *cli.Command {
	return &cli.Command{
		Name:      "send",
		Usage:     "Type a prompt into a session's agent and press Enter",
		UsageText: "hive session send <id|name> <text...> [--no-submit] [--json]\n   echo 'multi-line prompt' | hive session send <id|name> -",
		Description: `Types text into the agent pane of a session, waits for the agent to finish
rendering it, presses Enter once, and prints the screen after.

The agent pane is found by status detection, not by the tmux session's active
window, so this works while the shell window is focused. Read the screen to
see whether the agent took the prompt; 'hive session keys <id> Enter' presses
Enter again.

Pass '-' as the text to read it from stdin.`,
		Flags:  cmd.driveFlags(true),
		Action: cmd.runSend,
	}
}

func (cmd *SessionCmd) runSend(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) < 2 {
		return errors.New("usage: hive session send <id|name> <text...>")
	}
	sess, err := resolveSession(ctx, cmd.app.Sessions(), args[0])
	if err != nil {
		return err
	}
	text := strings.Join(args[1:], " ")
	if text == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read prompt from stdin: %w", err)
		}
		text = strings.TrimRight(string(data), "\n")
	}
	peek, err := cmd.app.Prompts().Send(ctx, sess, text, prompt.SendOptions{NoSubmit: cmd.drive.noSubmit, Lines: cmd.drive.lines})
	if err != nil {
		return fmt.Errorf("send to %s: %w", sess.Name, err)
	}
	return cmd.writePeek(c, sess, peek)
}

func (cmd *SessionCmd) keysCmd() *cli.Command {
	return &cli.Command{
		Name:      "keys",
		Usage:     "Press keys in a session's agent pane",
		UsageText: "hive session keys <id|name> <key...> [--json]",
		Description: `Presses tmux named keys in the agent pane of a session, in order, and prints
the screen after: '1 Enter' to pick a dialog option, Escape to dismiss a menu,
C-c to interrupt.`,
		Flags:  cmd.driveFlags(false),
		Action: cmd.runKeys,
	}
}

func (cmd *SessionCmd) runKeys(ctx context.Context, c *cli.Command) error {
	args := c.Args().Slice()
	if len(args) < 2 {
		return errors.New("usage: hive session keys <id|name> <key...>")
	}
	sess, err := resolveSession(ctx, cmd.app.Sessions(), args[0])
	if err != nil {
		return err
	}
	peek, err := cmd.app.Prompts().SendKeys(ctx, sess, args[1:], cmd.drive.lines)
	if err != nil {
		return fmt.Errorf("send keys to %s: %w", sess.Name, err)
	}
	return cmd.writePeek(c, sess, peek)
}

func (cmd *SessionCmd) peekCmd() *cli.Command {
	return &cli.Command{
		Name:      "peek",
		Usage:     "Show a session's agent state, context use, and screen",
		UsageText: "hive session peek <id|name> [--lines N] [--json]",
		Description: `Captures the agent pane of a session once. The state is a hint from status
detection (active, approval, question, ready, unknown, missing), read without
the TUI's smoothing; the screen is the source of truth.`,
		Flags:  cmd.driveFlags(false),
		Action: cmd.runPeek,
	}
}

func (cmd *SessionCmd) runPeek(ctx context.Context, c *cli.Command) error {
	key := c.Args().First()
	if key == "" {
		return errors.New("usage: hive session peek <id|name>")
	}
	sess, err := resolveSession(ctx, cmd.app.Sessions(), key)
	if err != nil {
		return err
	}
	peek, err := cmd.app.Prompts().Peek(ctx, sess, cmd.drive.lines)
	if err != nil {
		return fmt.Errorf("peek %s: %w", sess.Name, err)
	}
	return cmd.writePeek(c, sess, peek)
}

func (cmd *SessionCmd) writePeek(c *cli.Command, sess session.Session, peek prompt.SessionPeek) error {
	out := c.Root().Writer
	if cmd.drive.json {
		return iojson.WriteLine(out, peek)
	}
	_, _ = fmt.Fprintf(out, "session: %s (%s)\n", sess.Name, sess.ID)
	_, _ = fmt.Fprintf(out, "state:   %s\n", peek.State)
	if peek.Context != nil {
		_, _ = fmt.Fprintf(out, "context: %s/%s (%d%%)\n", peek.Context.Used, peek.Context.Total, peek.Context.Percent)
	}
	if peek.Tail != "" {
		_, _ = fmt.Fprintf(out, "---\n%s\n", peek.Tail)
	}
	return nil
}

type sessionLookup interface {
	GetSession(ctx context.Context, id string) (session.Session, error)
	ListSessions(ctx context.Context) ([]session.Session, error)
}

// resolveSession tries the ID, then an exact name or slug among active sessions.
func resolveSession(ctx context.Context, sessions sessionLookup, key string) (session.Session, error) {
	if sess, err := sessions.GetSession(ctx, key); err == nil {
		return sess, nil
	} else if !errors.Is(err, session.ErrNotFound) {
		return session.Session{}, fmt.Errorf("get session: %w", err)
	}

	all, err := sessions.ListSessions(ctx)
	if err != nil {
		return session.Session{}, fmt.Errorf("list sessions: %w", err)
	}
	var matches []session.Session
	for _, s := range all {
		if s.State == session.StateActive && (s.Name == key || s.Slug == key) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return session.Session{}, fmt.Errorf("no active session with ID, name, or slug %q", key)
	case 1:
		return matches[0], nil
	default:
		return session.Session{}, fmt.Errorf("%d active sessions are named %q; use the session ID", len(matches), key)
	}
}
