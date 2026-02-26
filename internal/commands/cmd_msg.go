package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/session"
	"github.com/colonyops/hive/internal/hive"
	"github.com/colonyops/hive/pkg/iojson"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/urfave/cli/v3"
)

func (cmd *MsgCmd) messages() *hive.MessageService {
	return cmd.app.Messages
}

type MsgCmd struct {
	flags *Flags
	app   *hive.App

	// pub flags
	pubTopics  []string
	pubFile    string
	pubSender  string
	pubMessage string

	// sub flags
	subTopic   string
	subTimeout string
	subTail    int
	subListen  bool
	subWait    bool
	subAck     bool

	// inbox flags
	inboxAll     bool
	inboxAck     bool
	inboxSession string
	inboxListen  bool
	inboxWait    bool
	inboxTimeout string
	inboxTail    int

	// topic flags
	topicNew    bool
	topicPrefix string
}

// NewMsgCmd creates a new msg command.
func NewMsgCmd(flags *Flags, app *hive.App) *MsgCmd {
	return &MsgCmd{flags: flags, app: app}
}

// Register adds the msg command to the application.
func (cmd *MsgCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "msg",
		Usage: "Publish and subscribe to inter-agent messages",
		Description: `Message commands for inter-agent communication.

Messages are stored in a SQLite database at $XDG_DATA_HOME/hive/.
Each topic is a named channel, allowing agents to communicate via pub/sub.

The sender is auto-detected from the current working directory's hive session.

Output format: All commands produce JSON Lines (one JSON object per line) on stdout.
Warnings and errors are written to stderr.`,
		Commands: []*cli.Command{
			cmd.pubCmd(),
			cmd.subCmd(),
			cmd.inboxCmd(),
			cmd.listCmd(),
			cmd.topicCmd(),
		},
	})

	return app
}

func (cmd *MsgCmd) pubCmd() *cli.Command {
	return &cli.Command{
		Name:      "pub",
		Usage:     "Publish a message to topic(s)",
		UsageText: "hive msg pub --topic <topic> [--topic <topic2>] [-m message | message | -f file | stdin]",
		Description: `Publishes a message to the specified topic(s).

The message can be provided as:
- Inline with -m/--message flag (recommended)
- A positional command-line argument
- From a file with -f/--file
- From stdin if no other source is provided

Only one message source may be used. An error is returned if multiple are provided.

The sender is auto-detected from the current hive session, or can be overridden with --sender.
Topic supports wildcards for publishing to multiple topics (e.g., agent.*.inbox).

Output: JSON confirmation line with status, resolved topics, and sender.

Examples:
  hive msg pub --topic build.started -m "Build starting"
  hive msg pub -t agent.abc.inbox -t agent.xyz.inbox -m "Hello all"
  hive msg pub -t "agent.*.inbox" -m "Broadcast message"
  echo "Hello" | hive msg pub --topic greetings
  hive msg pub --topic logs -f build.log`,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:        "topic",
				Aliases:     []string{"t"},
				Usage:       "topic(s) to publish to (supports wildcards, repeatable)",
				Required:    true,
				Destination: &cmd.pubTopics,
			},
			&cli.StringFlag{
				Name:        "message",
				Aliases:     []string{"m"},
				Usage:       "inline message content",
				Destination: &cmd.pubMessage,
			},
			&cli.StringFlag{
				Name:        "file",
				Aliases:     []string{"f"},
				Usage:       "read message from file",
				Destination: &cmd.pubFile,
			},
			&cli.StringFlag{
				Name:        "sender",
				Aliases:     []string{"s"},
				Usage:       "override sender ID (default: auto-detect from session)",
				Destination: &cmd.pubSender,
			},
		},
		Action: cmd.runPub,
	}
}

func (cmd *MsgCmd) subCmd() *cli.Command {
	return &cli.Command{
		Name:      "sub",
		Usage:     "Read messages from a topic",
		UsageText: "hive msg sub [--topic <pattern>] [--tail N] [--listen] [--ack]",
		Description: `Reads messages from topics, optionally filtering by topic pattern.

By default, returns all messages as JSON Lines and exits without acknowledging.
Use --ack to mark messages as read. Use --listen to poll for new messages,
or --wait to block until a single message arrives (useful for inter-agent handoff).

For unread inbox messages, use "hive msg inbox" instead.

Topic patterns:
- No topic or "*": all messages
- "exact.topic": exact topic match
- "prefix.*": wildcard match for topics starting with "prefix."

Output: One JSON object per line (JSON Lines format).
On timeout (--listen/--wait), prints a JSON status line to stdout and exits with code 1.

Examples:
  hive msg sub                       # all messages as JSON
  hive msg sub --topic agent.build   # specific topic
  hive msg sub --topic agent.*       # wildcard pattern
  hive msg sub --tail 10             # last 10 messages
  hive msg sub --listen              # poll for new messages
  hive msg sub --wait --topic handoff # wait for single message
  hive msg sub --ack                 # read and acknowledge`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "topic",
				Aliases:     []string{"t"},
				Usage:       "topic pattern to subscribe to (supports wildcards like agent.*)",
				Destination: &cmd.subTopic,
			},
			&cli.IntFlag{
				Name:        "tail",
				Aliases:     []string{"n", "last"},
				Usage:       "return only last N messages",
				Destination: &cmd.subTail,
			},
			&cli.BoolFlag{
				Name:        "listen",
				Aliases:     []string{"l"},
				Usage:       "poll for new messages instead of returning immediately",
				Destination: &cmd.subListen,
			},
			&cli.BoolFlag{
				Name:        "wait",
				Aliases:     []string{"w"},
				Usage:       "wait for a single message and exit (for inter-agent handoff)",
				Destination: &cmd.subWait,
			},
			&cli.BoolFlag{
				Name:        "ack",
				Usage:       "acknowledge (mark as read) messages after reading",
				Destination: &cmd.subAck,
			},
			&cli.StringFlag{
				Name:        "timeout",
				Usage:       "timeout for --listen/--wait mode (e.g., 30s, 5m, 24h)",
				Value:       "30s",
				Destination: &cmd.subTimeout,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return cmd.runSub(ctx, c)
		},
	}
}

func (cmd *MsgCmd) listCmd() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "List all topics with activity info",
		UsageText: "hive msg list",
		Description: `Lists all topics with message counts, activity timestamps, and unread counts.

Output: One JSON object per line (JSON Lines format).
Fields: name, message_count, unread_count, last_activity, last_sender.
unread_count is included when running from a hive session directory.

Examples:
  hive msg list`,
		Action: cmd.runList,
	}
}

func (cmd *MsgCmd) topicCmd() *cli.Command {
	return &cli.Command{
		Name:      "topic",
		Usage:     "Generate a random topic ID",
		UsageText: "hive msg topic [--prefix <prefix>]",
		Description: `Generates a random topic ID for inter-agent communication.

The generated topic ID follows the format "<prefix>.<4-char-alphanumeric>".
The prefix defaults to "agent" but can be configured via messaging.topic_prefix
in your config file, or overridden with --prefix.

Output: Plain text topic ID.

Examples:
  hive msg topic              # outputs: agent.x7k2 (using config prefix)
  hive msg topic --prefix task   # outputs: task.x7k2
  hive msg topic --prefix ""     # outputs: x7k2 (no prefix)`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "new",
				Aliases:     []string{"n"},
				Usage:       "generate a new random topic ID (default behavior)",
				Destination: &cmd.topicNew,
			},
			&cli.StringFlag{
				Name:        "prefix",
				Aliases:     []string{"p"},
				Usage:       "topic prefix (overrides config, use empty string for no prefix)",
				Destination: &cmd.topicPrefix,
			},
		},
		Action: cmd.runTopic,
	}
}

func (cmd *MsgCmd) runTopic(_ context.Context, c *cli.Command) error {
	// Determine prefix: flag override > config > default "agent"
	prefix := cmd.app.Config.Messaging.TopicPrefix
	if c.IsSet("prefix") {
		prefix = cmd.topicPrefix
	}

	// Generate topic ID
	id := randid.Generate(4)
	var topicID string
	if prefix != "" {
		topicID = prefix + "." + id
	} else {
		topicID = id
	}

	_, err := fmt.Fprintln(c.Root().Writer, topicID)
	return err
}

func (cmd *MsgCmd) runPub(ctx context.Context, c *cli.Command) error {
	msgs := cmd.messages()

	topics := cmd.pubTopics
	if len(topics) == 0 {
		return fmt.Errorf("at least one topic required")
	}

	// Determine message content — exactly one source allowed
	payload, err := cmd.resolvePayload(c)
	if err != nil {
		return err
	}

	// Auto-detect session and set sender
	sessionID, _ := cmd.detectSessionID(ctx) // Best-effort detection for sender
	sender := cmd.pubSender
	if sessionID != "" && sender == "" {
		sender = sessionID // Auto-set sender = session_id
	}

	msg := messaging.Message{
		Payload:   payload,
		Sender:    sender,
		SessionID: sessionID,
	}

	result, err := msgs.Publish(ctx, msg, topics)
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	// Print confirmation
	type pubConfirmation struct {
		Status string   `json:"status"`
		Topics []string `json:"topics"`
		Sender string   `json:"sender,omitempty"`
	}
	confirmation := pubConfirmation{
		Status: "ok",
		Topics: result.Topics,
		Sender: sender,
	}
	return iojson.WriteLine(c.Root().Writer, confirmation)
}

// resolvePayload determines the message content from exactly one source.
func (cmd *MsgCmd) resolvePayload(c *cli.Command) (string, error) {
	sources := 0
	if cmd.pubMessage != "" {
		sources++
	}
	if c.NArg() >= 1 {
		sources++
	}
	if cmd.pubFile != "" {
		sources++
	}

	if sources > 1 {
		return "", fmt.Errorf("multiple message sources provided; use only one of: -m flag, positional argument, -f file, or stdin")
	}

	switch {
	case cmd.pubMessage != "":
		return cmd.pubMessage, nil
	case c.NArg() >= 1:
		return c.Args().Get(0), nil
	case cmd.pubFile != "":
		data, err := os.ReadFile(cmd.pubFile)
		if err != nil {
			return "", fmt.Errorf("read file: %w", err)
		}
		return string(data), nil
	default:
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(data), nil
	}
}

func (cmd *MsgCmd) runSub(ctx context.Context, c *cli.Command) error {
	msgs := cmd.messages()

	topic := cmd.subTopic
	if topic == "" {
		topic = "*"
	}

	// Wait mode: wait for a single message and exit
	if cmd.subWait {
		return cmd.waitForMessage(ctx, c, msgs, topic, cmd.subAck)
	}

	// Listen mode: poll for new messages
	if cmd.subListen {
		return cmd.listenForMessages(ctx, c, msgs, topic, cmd.subAck)
	}

	// Default: return messages immediately
	messages, err := msgs.Subscribe(ctx, topic, time.Time{})
	if err != nil {
		if errors.Is(err, messaging.ErrTopicNotFound) {
			return nil // No messages, no output
		}
		return fmt.Errorf("subscribe: %w", err)
	}

	// Apply --tail N limit if specified
	if cmd.subTail > 0 && len(messages) > cmd.subTail {
		messages = messages[len(messages)-cmd.subTail:]
	}

	if err := cmd.printMessages(c.Root().Writer, messages); err != nil {
		return err
	}

	// Acknowledge only when --ack is set
	if cmd.subAck && len(messages) > 0 {
		cmd.acknowledgeMessages(ctx, msgs, messages)
	}

	return nil
}

func (cmd *MsgCmd) listenForMessages(ctx context.Context, c *cli.Command, msgs *hive.MessageService, topic string, ack bool) error {
	timeout, err := time.ParseDuration(cmd.subTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	deadline := time.Now().Add(timeout)
	since := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return cmd.handleTimeout(c, topic, timeout)
			}

			messages, err := msgs.Subscribe(ctx, topic, since)
			if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
				return fmt.Errorf("subscribe: %w", err)
			}

			if len(messages) > 0 {
				if err := cmd.printMessages(c.Root().Writer, messages); err != nil {
					return err
				}
				since = messages[len(messages)-1].CreatedAt

				if ack {
					cmd.acknowledgeMessages(ctx, msgs, messages)
				}
			}
		}
	}
}

func (cmd *MsgCmd) waitForMessage(ctx context.Context, c *cli.Command, msgs *hive.MessageService, topic string, ack bool) error {
	// Use 24h default for --wait mode (essentially forever for handoff scenarios)
	timeout := 24 * time.Hour
	if cmd.subTimeout != "30s" { // User explicitly set a timeout
		var err error
		timeout, err = time.ParseDuration(cmd.subTimeout)
		if err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}

	deadline := time.Now().Add(timeout)
	since := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return cmd.handleTimeout(c, topic, timeout)
			}

			messages, err := msgs.Subscribe(ctx, topic, since)
			if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
				return fmt.Errorf("subscribe: %w", err)
			}

			if len(messages) > 0 {
				// Return only the first message
				firstMsg := messages[:1]
				if err := cmd.printMessages(c.Root().Writer, firstMsg); err != nil {
					return err
				}

				if ack {
					cmd.acknowledgeMessages(ctx, msgs, firstMsg)
				}
				return nil
			}
		}
	}
}

// handleTimeout prints a JSON status line and returns a non-zero exit.
func (cmd *MsgCmd) handleTimeout(c *cli.Command, topic string, duration time.Duration) error {
	type timeoutStatus struct {
		Status   string `json:"status"`
		Topic    string `json:"topic"`
		Duration string `json:"duration"`
	}
	_ = iojson.WriteLine(c.Root().Writer, timeoutStatus{
		Status:   "timeout",
		Topic:    topic,
		Duration: duration.String(),
	})
	return cli.Exit("", 1)
}

func (cmd *MsgCmd) runList(ctx context.Context, c *cli.Command) error {
	msgs := cmd.messages()

	topics, err := msgs.ListTopics(ctx)
	if err != nil {
		return fmt.Errorf("list topics: %w", err)
	}

	if len(topics) == 0 {
		return nil // No topics, no output
	}

	// Try to detect session for unread counts (best-effort)
	sessionID, _ := cmd.detectSessionID(ctx)

	type topicInfo struct {
		Name         string `json:"name"`
		MessageCount int    `json:"message_count"`
		UnreadCount  int    `json:"unread_count"`
		LastActivity string `json:"last_activity,omitempty"`
		LastSender   string `json:"last_sender,omitempty"`
	}

	var infos []topicInfo
	for _, t := range topics {
		info := topicInfo{Name: t}

		messages, err := msgs.Subscribe(ctx, t, time.Time{})
		if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
			return fmt.Errorf("get messages for topic %s: %w", t, err)
		}
		info.MessageCount = len(messages)

		if len(messages) > 0 {
			last := messages[len(messages)-1]
			info.LastActivity = last.CreatedAt.UTC().Format(time.RFC3339)
			info.LastSender = last.Sender
		}

		if sessionID != "" {
			unread, err := msgs.GetUnread(ctx, sessionID, t)
			if err == nil {
				info.UnreadCount = len(unread)
			}
		}

		infos = append(infos, info)
	}

	for _, info := range infos {
		if err := iojson.WriteLine(c.Root().Writer, info); err != nil {
			return err
		}
	}
	return nil
}

func (cmd *MsgCmd) detectSessionID(ctx context.Context) (string, error) {
	return cmd.app.Sessions.DetectSession(ctx)
}

// resolveSessionID resolves a session from a --session flag value (ID or name).
// Returns the session ID if found, or an error.
func (cmd *MsgCmd) resolveSessionID(ctx context.Context, sessionRef string) (string, error) {
	// Try direct ID lookup first
	sess, err := cmd.app.Sessions.GetSession(ctx, sessionRef)
	if err == nil {
		return sess.ID, nil
	}

	// Fall back to name match
	sessions, err := cmd.app.Sessions.ListSessions(ctx)
	if err != nil {
		return "", fmt.Errorf("list sessions: %w", err)
	}
	for _, s := range sessions {
		if s.State != session.StateActive {
			continue
		}
		if s.Name == sessionRef || s.Slug == sessionRef {
			return s.ID, nil
		}
	}

	return "", fmt.Errorf("session not found: %s", sessionRef)
}

func (cmd *MsgCmd) printMessages(w io.Writer, messages []messaging.Message) error {
	for _, msg := range messages {
		if err := iojson.WriteLine(w, msg); err != nil {
			return err
		}
	}
	return nil
}

// acknowledgeMessages marks messages as read by the current session.
// Logs errors but does not fail the operation.
func (cmd *MsgCmd) acknowledgeMessages(ctx context.Context, msgs *hive.MessageService, messages []messaging.Message) {
	sessionID, err := cmd.detectSessionID(ctx)
	if err != nil {
		log.Printf("warning: failed to detect session for acknowledgment: %v", err)
		return
	}
	if sessionID == "" {
		return // Not in a session, skip acknowledgment
	}

	messageIDs := make([]string, len(messages))
	for i, msg := range messages {
		messageIDs[i] = msg.ID
	}
	if err := msgs.Acknowledge(ctx, sessionID, messageIDs); err != nil {
		log.Printf("warning: failed to acknowledge %d messages: %v", len(messageIDs), err)
	}
}

func (cmd *MsgCmd) inboxCmd() *cli.Command {
	return &cli.Command{
		Name:  "inbox",
		Usage: "Read messages from your session's inbox",
		Description: `Auto-resolves inbox topic (agent.<id>.inbox).
Shows unread messages by default (without acknowledging).
Use --ack to mark messages as read, or --all to see all messages.

The session is auto-detected from the working directory, or can be specified
with --session <id|name> to avoid detection issues.

Output: One JSON object per line (JSON Lines format).
On timeout (--listen/--wait), prints a JSON status line and exits with code 1.

Examples:
  hive msg inbox                         # unread, don't mark
  hive msg inbox --ack                   # unread, mark as read
  hive msg inbox --all                   # all messages
  hive msg inbox --session my-session    # specify session explicitly
  hive msg inbox --listen --timeout 30s  # poll for new messages
  hive msg inbox --wait                  # wait for single message
  hive msg inbox --tail 5               # last 5 unread messages`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "show all messages, not just unread",
				Destination: &cmd.inboxAll,
			},
			&cli.BoolFlag{
				Name:        "ack",
				Usage:       "acknowledge (mark as read) messages after reading",
				Destination: &cmd.inboxAck,
			},
			&cli.StringFlag{
				Name:        "session",
				Usage:       "session ID or name (overrides auto-detection from working directory)",
				Destination: &cmd.inboxSession,
			},
			&cli.BoolFlag{
				Name:        "listen",
				Aliases:     []string{"l"},
				Usage:       "poll for new messages instead of returning immediately",
				Destination: &cmd.inboxListen,
			},
			&cli.BoolFlag{
				Name:        "wait",
				Aliases:     []string{"w"},
				Usage:       "wait for a single message and exit",
				Destination: &cmd.inboxWait,
			},
			&cli.StringFlag{
				Name:        "timeout",
				Usage:       "timeout for --listen/--wait mode (e.g., 30s, 5m, 24h)",
				Value:       "30s",
				Destination: &cmd.inboxTimeout,
			},
			&cli.IntFlag{
				Name:        "tail",
				Aliases:     []string{"n"},
				Usage:       "return only last N messages",
				Destination: &cmd.inboxTail,
			},
		},
		Action: cmd.runInbox,
	}
}

func (cmd *MsgCmd) runInbox(ctx context.Context, c *cli.Command) error {
	msgs := cmd.messages()

	sessionID, err := cmd.resolveInboxSession(ctx)
	if err != nil {
		return err
	}

	inboxTopic := "agent." + sessionID + ".inbox"

	// Delegate to listen/wait modes if requested
	if cmd.inboxWait {
		// Override sub fields for shared logic
		cmd.subTimeout = cmd.inboxTimeout
		return cmd.waitForMessage(ctx, c, msgs, inboxTopic, cmd.inboxAck)
	}
	if cmd.inboxListen {
		cmd.subTimeout = cmd.inboxTimeout
		return cmd.listenForMessages(ctx, c, msgs, inboxTopic, cmd.inboxAck)
	}

	var messages []messaging.Message
	var msgErr error

	if cmd.inboxAll {
		messages, msgErr = msgs.Subscribe(ctx, inboxTopic, time.Time{})
	} else {
		messages, msgErr = msgs.GetUnread(ctx, sessionID, inboxTopic)
	}

	if msgErr != nil && !errors.Is(msgErr, messaging.ErrTopicNotFound) {
		return fmt.Errorf("get inbox: %w", msgErr)
	}

	// Apply --tail N limit
	if cmd.inboxTail > 0 && len(messages) > cmd.inboxTail {
		messages = messages[len(messages)-cmd.inboxTail:]
	}

	if err := cmd.printMessages(c.Root().Writer, messages); err != nil {
		return err
	}

	// Acknowledge only when --ack is set
	if cmd.inboxAck && len(messages) > 0 {
		cmd.acknowledgeMessages(ctx, msgs, messages)
	}

	return nil
}

// resolveInboxSession resolves the session ID for inbox operations.
// Uses --session flag if provided, otherwise falls back to CWD detection.
func (cmd *MsgCmd) resolveInboxSession(ctx context.Context) (string, error) {
	if cmd.inboxSession != "" {
		resolvedID, err := cmd.resolveSessionID(ctx, cmd.inboxSession)
		if err != nil {
			return "", fmt.Errorf("resolve --session %q: %w", cmd.inboxSession, err)
		}

		// Warn if CWD detection returns a different session
		if detected, _ := cmd.detectSessionID(ctx); detected != "" && detected != resolvedID {
			log.Printf("warning: --session %s overrides detected session %s", cmd.inboxSession, detected)
		}

		return resolvedID, nil
	}

	sessionID, err := cmd.detectSessionID(ctx)
	if err != nil {
		return "", fmt.Errorf("detect session: %w", err)
	}
	if sessionID == "" {
		return "", fmt.Errorf("could not detect session from working directory; use --session <id> or run 'hive session info' to check")
	}

	return sessionID, nil
}
