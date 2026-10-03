package hive

import (
	"fmt"

	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/notify"
	"github.com/colonyops/hive/internal/core/terminal"
)

// NotificationRouter maps domain events to user-facing notifications.
type NotificationRouter struct {
	bus *eventbus.EventBus
}

// NewNotificationRouter constructs a router for event-to-notification mappings.
func NewNotificationRouter(bus *eventbus.EventBus) *NotificationRouter {
	return &NotificationRouter{bus: bus}
}

// Register subscribes all supported event mappings.
func (r *NotificationRouter) Register() {
	if r == nil || r.bus == nil {
		return
	}

	r.bus.SubscribeSessionCorrupted(func(p eventbus.SessionCorruptedPayload) {
		if p.Session == nil {
			return
		}
		r.notifyf(notify.LevelWarning, "session %q marked corrupted", p.Session.Name)
	})

	r.bus.SubscribeSessionDeleted(func(p eventbus.SessionDeletedPayload) {
		r.notifyf(notify.LevelInfo, "session %s deleted", p.SessionID)
	})

	r.bus.SubscribeSessionRecycled(func(p eventbus.SessionRecycledPayload) {
		if p.Session == nil {
			return
		}
		r.notifyf(notify.LevelInfo, "session %q recycled", p.Session.Name)
	})

	r.bus.SubscribeAgentStatusChanged(func(p eventbus.AgentStatusChangedPayload) {
		if p.Session == nil {
			return
		}
		if p.NewStatus == terminal.StatusMissing {
			r.notifyf(notify.LevelWarning, "agent %q entered missing state", p.Session.Name)
		}
	})

	r.bus.SubscribeMessageReceived(func(p eventbus.MessageReceivedPayload) {
		r.notifyf(notify.LevelInfo, "message received on %s", p.Topic)
	})
}

func (r *NotificationRouter) notifyf(level notify.Level, format string, args ...any) {
	r.bus.PublishNotificationPublished(eventbus.NotificationPublishedPayload{
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	})
}
