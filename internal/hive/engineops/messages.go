package engineops

import (
	"context"
	"errors"
	"time"

	"github.com/colonyops/hive/internal/domain/messaging"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/internal/ops/messages"
)

// pollInterval is how often a waiting message_list reads its topic.
var pollInterval = 500 * time.Millisecond

func (h *Handlers) MessageCreate(ctx context.Context, in messages.MessageCreateIn) (messages.MessageCreateOut, error) {
	if len(in.Topics) == 0 {
		return messages.MessageCreateOut{}, ops.Errorf(ops.KindInvalid, "give at least one topic")
	}
	c, err := h.resolve(ctx, in.Caller)
	if err != nil {
		return messages.MessageCreateOut{}, err
	}
	sender := in.Sender
	if sender == "" {
		sender = c.ID
	}
	result, err := h.engine.Messages().Publish(ctx, messaging.Message{Payload: in.Payload, Sender: sender, SessionID: c.ID}, in.Topics)
	if err != nil {
		return messages.MessageCreateOut{}, opError(err, "publish")
	}
	return messages.MessageCreateOut{Status: "ok", Topics: result.Topics, Sender: sender}, nil
}

func (h *Handlers) MessageList(ctx context.Context, in messages.MessageListIn) (messages.MessageListOut, error) {
	var c caller
	var err error
	if in.Inbox || in.Unread {
		c, err = h.resolveAgent(ctx, in.Caller, "an inbox or unread read")
	} else {
		c, err = h.resolve(ctx, in.Caller)
	}
	if err != nil {
		return messages.MessageListOut{}, err
	}
	topic := in.Topic
	if in.Inbox {
		topic = InboxTopic(c.ID)
	}
	if topic == "" {
		topic = "*"
	}
	if in.Wait > 0 {
		return h.waitForMessage(ctx, topic, time.Duration(in.Wait)*time.Second)
	}

	var list []messaging.Message
	if in.Unread {
		list, err = h.engine.Messages().GetUnread(ctx, c.ID, topic)
	} else {
		list, err = h.engine.Messages().Subscribe(ctx, topic, time.Time{})
	}
	if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
		return messages.MessageListOut{}, opError(err, "read messages")
	}
	if in.Tail > 0 && len(list) > in.Tail {
		list = list[len(list)-in.Tail:]
	}
	return messages.MessageListOut{Messages: nonNil(list)}, nil
}

// InboxTopic is the topic a session's or chat's inbox reads.
func InboxTopic(id string) string { return "agent." + id + ".inbox" }

// waitForMessage answers the first message published to topic after the call
// starts.
func (h *Handlers) waitForMessage(ctx context.Context, topic string, wait time.Duration) (messages.MessageListOut, error) {
	since := time.Now()
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return messages.MessageListOut{}, ops.Errorf(ops.KindUnavailable, "wait for a message on %s: %v", topic, ctx.Err())
		case <-deadline.C:
			return messages.MessageListOut{Messages: []messaging.Message{}, TimedOut: true, Topic: topic, Wait: wait}, nil
		case <-ticker.C:
			list, err := h.engine.Messages().Subscribe(ctx, topic, since)
			if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
				return messages.MessageListOut{}, opError(err, "read messages")
			}
			if len(list) > 0 {
				return messages.MessageListOut{Messages: list[:1]}, nil
			}
		}
	}
}

func (h *Handlers) MessageReadPut(ctx context.Context, in messages.MessageReadPutIn) (messages.MessageReadPutOut, error) {
	if len(in.MessageIDs) == 0 {
		return messages.MessageReadPutOut{}, ops.Errorf(ops.KindInvalid, "give at least one message id")
	}
	c, err := h.resolveAgent(ctx, in.Caller, "message_read_put")
	if err != nil {
		return messages.MessageReadPutOut{}, err
	}
	if err := h.engine.Messages().Acknowledge(ctx, c.ID, in.MessageIDs); err != nil {
		return messages.MessageReadPutOut{}, opError(err, "mark read")
	}
	return messages.MessageReadPutOut{MessageIDs: in.MessageIDs, Reader: c.ID}, nil
}

func (h *Handlers) TopicList(ctx context.Context, in messages.TopicListIn) (messages.TopicListOut, error) {
	c, err := h.resolve(ctx, in.Caller)
	if err != nil {
		return messages.TopicListOut{}, err
	}
	names, err := h.engine.Messages().ListTopics(ctx)
	if err != nil {
		return messages.TopicListOut{}, opError(err, "list topics")
	}
	out := messages.TopicListOut{Topics: make([]messages.TopicOut, 0, len(names))}
	for _, name := range names {
		info := messages.TopicOut{Name: name}
		list, err := h.engine.Messages().Subscribe(ctx, name, time.Time{})
		if err != nil && !errors.Is(err, messaging.ErrTopicNotFound) {
			return messages.TopicListOut{}, opError(err, "read topic "+name)
		}
		info.MessageCount = len(list)
		if len(list) > 0 {
			last := list[len(list)-1]
			info.LastActivity = last.CreatedAt.UTC().Format(time.RFC3339)
			info.LastSender = last.Sender
		}
		if c.ID != "" {
			unread, err := h.engine.Messages().GetUnread(ctx, c.ID, name)
			if err != nil {
				h.log.Debug().Err(err).Str("topic", name).Msg("unread count")
			}
			info.UnreadCount = len(unread)
		}
		out.Topics = append(out.Topics, info)
	}
	return out, nil
}
