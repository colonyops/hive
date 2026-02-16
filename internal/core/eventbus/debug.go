package eventbus

import "github.com/rs/zerolog"

// RegisterDebugLogger subscribes to all event types and logs them at debug level.
// Subscriber callbacks are always invoked; zerolog short-circuits disabled levels
// before formatting fields, so the formatting overhead is minimal above debug.
func RegisterDebugLogger(bus *EventBus, logger zerolog.Logger) {
	bus.SubscribeSessionCreated(func(p SessionCreatedPayload) {
		logger.Debug().
			Str("event", string(EventSessionCreated)).
			Str("session_id", p.Session.ID).
			Str("session_name", p.Session.Name).
			Msg("event fired")
	})

	bus.SubscribeSessionRecycled(func(p SessionRecycledPayload) {
		logger.Debug().
			Str("event", string(EventSessionRecycled)).
			Str("session_id", p.Session.ID).
			Msg("event fired")
	})

	bus.SubscribeSessionDeleted(func(p SessionDeletedPayload) {
		logger.Debug().
			Str("event", string(EventSessionDeleted)).
			Str("session_id", p.SessionID).
			Msg("event fired")
	})

	bus.SubscribeSessionRenamed(func(p SessionRenamedPayload) {
		logger.Debug().
			Str("event", string(EventSessionRenamed)).
			Str("session_id", p.Session.ID).
			Str("old_name", p.OldName).
			Str("new_name", p.Session.Name).
			Msg("event fired")
	})

	bus.SubscribeSessionCorrupted(func(p SessionCorruptedPayload) {
		logger.Debug().
			Str("event", string(EventSessionCorrupted)).
			Str("session_id", p.Session.ID).
			Msg("event fired")
	})

	bus.SubscribeAgentStatusChanged(func(p AgentStatusChangedPayload) {
		logger.Debug().
			Str("event", string(EventAgentStatusChanged)).
			Str("session_id", p.Session.ID).
			Str("old_status", string(p.OldStatus)).
			Str("new_status", string(p.NewStatus)).
			Msg("event fired")
	})

	bus.SubscribeMessageReceived(func(p MessageReceivedPayload) {
		logger.Debug().
			Str("event", string(EventMessageReceived)).
			Str("topic", p.Topic).
			Msg("event fired")
	})

	bus.SubscribeTuiStarted(func(_ TUIStartedPayload) {
		logger.Debug().
			Str("event", string(EventTuiStarted)).
			Msg("event fired")
	})

	bus.SubscribeTuiStopped(func(_ TUIStoppedPayload) {
		logger.Debug().
			Str("event", string(EventTuiStopped)).
			Msg("event fired")
	})

	bus.SubscribeConfigReloaded(func(_ ConfigReloadedPayload) {
		logger.Debug().
			Str("event", string(EventConfigReloaded)).
			Msg("event fired")
	})
}
