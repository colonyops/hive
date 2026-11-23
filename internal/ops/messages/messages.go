// Package messages declares the inter-agent messaging operations: publish to
// topics, read them, and mark messages read. They run on the engine.
package messages

import (
	"time"

	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/ops"
)

var Toolset = ops.Toolset{Name: "messages", MCPPath: "/mcp"}

func All() []ops.Op {
	return []ops.Op{MessageCreate, MessageList, MessageReadPut, TopicList}
}

// MaxWait bounds message_list's wait: a day, the CLI's longest wait.
const MaxWait = 86400

type MessageCreateIn struct {
	Caller  string   `json:"caller"           jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	Topics  []string `json:"topics"           jsonschema:"The topics to publish to. A topic may hold a * wildcard, such as agent.*.inbox, to reach every topic it matches."`
	Payload string   `json:"payload"          jsonschema:"The message."`
	Sender  string   `json:"sender,omitempty" jsonschema:"Who the message is from. Defaults to the caller's session or chat id."`
}

var MessageCreate = ops.Spec[MessageCreateIn]{
	Name: "message.create", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Publish a message to topics",
	Guidance: "Store the message on every topic named and answer with the topics it reached. Another agent's inbox " +
		"is agent.<session-id>.inbox.",
}

type MessageListIn struct {
	Caller string `json:"caller"           jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
	Topic  string `json:"topic,omitempty"  jsonschema:"An exact topic, a prefix.* pattern, or * (default) for every topic."`
	Inbox  bool   `json:"inbox,omitempty"  jsonschema:"Read the caller's own inbox, agent.<id>.inbox, instead of topic."`
	Unread bool   `json:"unread,omitempty" jsonschema:"Only messages the caller has not marked read. Needs a session or chat caller."`
	Tail   int    `json:"tail,omitempty"   jsonschema:"Only the last n messages."`
	Wait   int    `json:"wait,omitempty"   jsonschema:"Seconds to wait for the first message that arrives after the call starts. 0 (default) answers at once."`
}

var MessageList = ops.Spec[MessageListIn]{
	Name: "message.list", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary: "Read messages from a topic or your inbox",
	Guidance: "Answer with the messages, oldest first. Reading marks nothing read; call message_read_put with the ids " +
		"you handled. With wait, answer with the first new message, or with none and timedOut when the wait ends. " +
		"Keep a wait short over MCP: a client may give up on a long call.",
	Fields: ops.Constraints{"tail": {Min: ptr(0)}, "wait": ops.Range(0, MaxWait)},
}

type MessageReadPutIn struct {
	Caller     string   `json:"caller"     jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory. Never guess or reuse another value."`
	MessageIDs []string `json:"messageIds" jsonschema:"The ids of the messages to mark read."`
}

var MessageReadPut = ops.Spec[MessageReadPutIn]{
	Name: "message_read.put", Owner: ops.OwnerEngine, Effect: ops.EffectWrite, Toolset: Toolset, Caller: ops.CallerRuleSession,
	Summary:  "Mark messages read",
	Args:     []string{"messageIds"},
	Guidance: "Mark the messages read for the caller, so an unread read skips them, and answer with the ids and the reader.",
}

type TopicListIn struct {
	Caller string `json:"caller" jsonschema:"Who is calling. In a Hive chat it is this process's HIVE_AGENT_SESSION environment variable. In a hive session it is the absolute path of your working directory; outside one it is global. Never guess or reuse another value."`
}

var TopicList = ops.Spec[TopicListIn]{
	Name: "topic.list", Owner: ops.OwnerEngine, Effect: ops.EffectRead, Toolset: Toolset, Caller: ops.CallerRuleSessionOrGlobal,
	Summary:  "List message topics",
	Guidance: "Answer with every topic, its message count and last activity, and its unread count for a session or chat caller.",
}

type MessageCreateOut struct {
	Status string   `json:"status"`
	Topics []string `json:"topics"`
	Sender string   `json:"sender,omitempty"`
}

type MessageListOut struct {
	Messages []messaging.Message `json:"messages"`
	TimedOut bool                `json:"timedOut,omitempty"`
	// Topic and Wait echo a timed-out wait for its CLI status line.
	Topic string        `json:"topic,omitempty"`
	Wait  time.Duration `json:"-"`
}

// JSONLines prints each message, or the status line of a wait that timed out.
func (o MessageListOut) JSONLines() []any {
	if o.TimedOut {
		return []any{TimeoutLine{Status: "timeout", Topic: o.Topic, Duration: o.Wait.String()}}
	}
	out := make([]any, len(o.Messages))
	for i, m := range o.Messages {
		out[i] = m
	}
	return out
}

// TimeoutLine is what the CLI prints when a wait ends with no message.
type TimeoutLine struct {
	Status   string `json:"status"`
	Topic    string `json:"topic"`
	Duration string `json:"duration"`
}

type MessageReadPutOut struct {
	MessageIDs []string `json:"messageIds"`
	Reader     string   `json:"reader"`
}

type TopicOut struct {
	Name         string `json:"name"`
	MessageCount int    `json:"message_count"`
	UnreadCount  int    `json:"unread_count"`
	LastActivity string `json:"last_activity,omitempty"`
	LastSender   string `json:"last_sender,omitempty"`
}

type TopicListOut struct {
	Topics []TopicOut `json:"topics"`
}

func (o TopicListOut) JSONLines() []any {
	out := make([]any, len(o.Topics))
	for i, t := range o.Topics {
		out[i] = t
	}
	return out
}

func ptr(f float64) *float64 { return &f }
