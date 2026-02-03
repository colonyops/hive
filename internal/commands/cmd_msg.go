package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
	"github.com/hay-kot/hive/pkg/randid"
	"github.com/urfave/cli/v3"
)

type MsgCmd struct {
	flags *Flags

	// pub flags
	pubTopics []string
	pubFile   string
	pubSender string

	// sub flags
	subTopic   string
	subTimeout string
	subLast    int
	subListen  bool
	subWait    bool
	subPeek    bool

	// inbox flags
	inboxAll  bool
	inboxPeek bool

	// topic flags
	topicNew    bool
	topicPrefix string
}

// NewMsgCmd creates a new msg command.
func NewMsgCmd(flags *Flags) *MsgCmd {
	return &MsgCmd{flags: flags}
}

// Register adds the msg command to the application.
func (cmd *MsgCmd) Register(app *cli.Command) *cli.Command {
	app.Commands = append(app.Commands, &cli.Command{
		Name:  "msg",
		Usage: "Publish and subscribe to inter-agent messages",
		Description: `Message commands for inter-agent communication.

Messages are stored in topic-based JSON files at $XDG_DATA_HOME/hive/messages/topics/.
Each topic is a separate file, allowing agents to communicate via named channels.

The sender is auto-detected from the current working directory's hive session.`,
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
		UsageText: "hive msg pub --topic <topic> [--topic <topic2>] [message]",
		Description: `Publishes a message to the specified topic(s).

The message can be provided as:
- A command-line argument
- From a file with -f/--file
- From stdin if no argument is provided

The sender is auto-detected from the current hive session, or can be overridden with --sender.
Topic supports wildcards for publishing to multiple topics (e.g., agent.*.inbox).

Examples:
  hive msg pub --topic build.started "Build starting"
  hive msg pub -t agent.abc.inbox -t agent.xyz.inbox "Hello all"
  hive msg pub -t "agent.*.inbox" "Broadcast message"
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
		UsageText: "hive msg sub [--topic <pattern>] [--last N] [--listen]",
		Description: `Reads messages from topics, optionally filtering by topic pattern.

By default, returns all messages as JSON and exits. Use --listen to poll for new messages,
or --wait to block until a single message arrives (useful for inter-agent handoff).

Messages are automatically acknowledged when read (if in a hive session).
Use --peek to read without acknowledging.

For unread inbox messages, use "hive msg inbox" instead.

Topic patterns:
- No topic or "*": all messages
- "exact.topic": exact topic match
- "prefix.*": wildcard match for topics starting with "prefix."

Examples:
  hive msg sub                       # all messages as JSON
  hive msg sub --topic agent.build   # specific topic
  hive msg sub --topic agent.*       # wildcard pattern
  hive msg sub --last 10             # last 10 messages
  hive msg sub --listen              # poll for new messages
  hive msg sub --wait --topic handoff # wait for single message
  hive msg sub --peek                # read without acknowledging`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "topic",
				Aliases:     []string{"t"},
				Usage:       "topic pattern to subscribe to (supports wildcards like agent.*)",
				Destination: &cmd.subTopic,
			},
			&cli.IntFlag{
				Name:        "last",
				Aliases:     []string{"n"},
				Usage:       "return only last N messages",
				Destination: &cmd.subLast,
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
				Name:        "peek",
				Usage:       "read without acknowledging messages",
				Destination: &cmd.subPeek,
			},
			&cli.StringFlag{
				Name:        "timeout",
				Usage:       "timeout for --listen/--wait mode (e.g., 30s, 5m, 24h)",
				Value:       "30s",
				Destination: &cmd.subTimeout,
			},
		},
		Action: cmd.runSub,
	}
}

func (cmd *MsgCmd) listCmd() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "List all topics",
		UsageText: "hive msg list",
		Description: `Lists all topics with their message counts as JSON.

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
	prefix := cmd.flags.Config.Messaging.TopicPrefix
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
	store := cmd.getMsgStore()

	topics := cmd.pubTopics
	if len(topics) == 0 {
		return fmt.Errorf("at least one topic required")
	}

	// Determine message content
	var payload string
	switch {
	case c.NArg() >= 1:
		payload = c.Args().Get(0)
	case cmd.pubFile != "":
		data, err := os.ReadFile(cmd.pubFile)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		payload = string(data)
	default:
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		payload = string(data)
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

	if err := store.Publish(ctx, msg, topics); err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	return nil
}

func (cmd *MsgCmd) runSub(ctx context.Context, c *cli.Command) error {
	store := cmd.getMsgStore()

	topic := cmd.subTopic
	if topic == "" {
		topic = "*"
	}

	// Wait mode: wait for a single message and exit
	if cmd.subWait {
		return cmd.waitForMessage(ctx, c, store, topic)
	}

	// Listen mode: poll for new messages
	if cmd.subListen {
		return cmd.listenForMessages(ctx, c, store, topic)
	}

	// Default: return messages immediately
	messages, err := store.Subscribe(ctx, topic, time.Time{})
	if err != nil {
		if errors.Is(err, messaging.ErrTopicNotFound) {
			return nil // No messages, no output
		}
		return fmt.Errorf("subscribe: %w", err)
	}

	// Apply --last N limit if specified
	if cmd.subLast > 0 && len(messages) > cmd.subLast {
		messages = messages[len(messages)-cmd.subLast:]
	}

	if err := cmd.printMessages(c.Root().Writer, messages); err != nil {
		return err
	}

	// Auto-acknowledge unless peeking
	if !cmd.subPeek && len(messages) > 0 {
		cmd.acknowledgeMessages(ctx, store, messages)
	}

	return nil
}

func (cmd *MsgCmd) listenForMessages(ctx context.Context, c *cli.Command, store messaging.Store, topic string) error {
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
				return nil // Timeout reached, exit silently
			}

			messages, err := store.Subscribe(ctx, topic, since)
			if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
				return fmt.Errorf("subscribe: %w", err)
			}

			if len(messages) > 0 {
				if err := cmd.printMessages(c.Root().Writer, messages); err != nil {
					return err
				}
				since = messages[len(messages)-1].CreatedAt

				// Auto-acknowledge unless peeking
				if !cmd.subPeek {
					cmd.acknowledgeMessages(ctx, store, messages)
				}
			}
		}
	}
}

func (cmd *MsgCmd) waitForMessage(ctx context.Context, c *cli.Command, store messaging.Store, topic string) error {
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
				return fmt.Errorf("timeout waiting for message on topic %q", topic)
			}

			messages, err := store.Subscribe(ctx, topic, since)
			if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
				return fmt.Errorf("subscribe: %w", err)
			}

			if len(messages) > 0 {
				// Return only the first message
				firstMsg := messages[:1]
				if err := cmd.printMessages(c.Root().Writer, firstMsg); err != nil {
					return err
				}

				// Auto-acknowledge unless peeking
				if !cmd.subPeek {
					cmd.acknowledgeMessages(ctx, store, firstMsg)
				}
				return nil
			}
		}
	}
}

func (cmd *MsgCmd) runList(ctx context.Context, c *cli.Command) error {
	store := cmd.getMsgStore()

	topics, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("list topics: %w", err)
	}

	if len(topics) == 0 {
		return nil // No topics, no output
	}

	// Get message counts for each topic
	type topicInfo struct {
		Name         string `json:"name"`
		MessageCount int    `json:"message_count"`
	}

	var infos []topicInfo
	for _, t := range topics {
		messages, err := store.Subscribe(ctx, t, time.Time{})
		if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
			return fmt.Errorf("get messages for topic %s: %w", t, err)
		}
		infos = append(infos, topicInfo{Name: t, MessageCount: len(messages)})
	}

	enc := json.NewEncoder(c.Root().Writer)
	for _, info := range infos {
		if err := enc.Encode(info); err != nil {
			return err
		}
	}
	return nil
}

func (cmd *MsgCmd) getMsgStore() messaging.Store {
	return cmd.flags.MsgStore
}

func (cmd *MsgCmd) detectSessionID(ctx context.Context) (string, error) {
	detector := messaging.NewSessionDetector(cmd.flags.Store)
	return detector.DetectSession(ctx)
}

func (cmd *MsgCmd) printMessages(w io.Writer, messages []messaging.Message) error {
	enc := json.NewEncoder(w)
	for _, msg := range messages {
		if err := enc.Encode(msg); err != nil {
			return err
		}
	}
	return nil
}

// acknowledgeMessages marks messages as read by the current session.
// Logs errors but does not fail the operation.
func (cmd *MsgCmd) acknowledgeMessages(ctx context.Context, store messaging.Store, messages []messaging.Message) {
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
	if err := store.Acknowledge(ctx, sessionID, messageIDs); err != nil {
		log.Printf("warning: failed to acknowledge %d messages: %v", len(messageIDs), err)
	}
}

func (cmd *MsgCmd) inboxCmd() *cli.Command {
	return &cli.Command{
		Name:  "inbox",
		Usage: "Read messages from your session's inbox",
		Description: `Auto-resolves inbox topic (agent.<id>.inbox).
Shows unread messages and marks as read by default.

Examples:
  hive msg inbox           # unread, mark as read
  hive msg inbox --peek    # unread, don't mark
  hive msg inbox --all     # all messages`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:        "all",
				Usage:       "show all messages, not just unread",
				Destination: &cmd.inboxAll,
			},
			&cli.BoolFlag{
				Name:        "peek",
				Usage:       "don't mark messages as read",
				Destination: &cmd.inboxPeek,
			},
		},
		Action: cmd.runInbox,
	}
}

func (cmd *MsgCmd) runInbox(ctx context.Context, c *cli.Command) error {
	store := cmd.getMsgStore()

	sessionID, err := cmd.detectSessionID(ctx)
	if err != nil {
		return fmt.Errorf("detect session: %w", err)
	}
	if sessionID == "" {
		return fmt.Errorf("not in a hive session (run from session working directory)")
	}

	inboxTopic := "agent." + sessionID + ".inbox"

	var messages []messaging.Message
	var msgErr error

	if cmd.inboxAll {
		messages, msgErr = store.Subscribe(ctx, inboxTopic, time.Time{})
	} else {
		messages, msgErr = store.GetUnread(ctx, sessionID, inboxTopic)
	}

	if msgErr != nil && !errors.Is(msgErr, messaging.ErrTopicNotFound) {
		return fmt.Errorf("get inbox: %w", msgErr)
	}

	if err := cmd.printMessages(c.Root().Writer, messages); err != nil {
		return err
	}

	// Auto-acknowledge unless peeking
	if !cmd.inboxPeek && len(messages) > 0 {
		cmd.acknowledgeMessages(ctx, store, messages)
	}

	return nil
}
