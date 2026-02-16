package eventbus

import (
	"fmt"

	"github.com/rs/zerolog"
)

// RegisterDebugLogger registers bus hooks that log all event activity at debug level.
// Uses OnPublish for event firing, OnDrop for buffer-full warnings, and OnPanic
// for subscriber panic reporting.
func RegisterDebugLogger(bus *EventBus, logger zerolog.Logger) {
	bus.OnPublish(func(event Event, _ any) {
		logger.Debug().Str("event", string(event)).Msg("event fired")
	})

	bus.OnDrop(func(event Event, _ any) {
		logger.Warn().Str("event", string(event)).Msg("event dropped: buffer full")
	})

	bus.OnPanic(func(event Event, _ any, recovered any) {
		logger.Error().
			Str("event", string(event)).
			Str("panic", fmt.Sprint(recovered)).
			Msg("subscriber panicked")
	})
}
