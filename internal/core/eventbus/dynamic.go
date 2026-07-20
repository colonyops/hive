package eventbus

import (
	"errors"
)

// ErrFull reports that an event could not be queued because the bus buffer is
// full. Callers that require delivery can return this error and retry.
var ErrFull = errors.New("event bus buffer full")

// Publish queues a dynamically named in-process event. Unlike the generated
// typed publishers, it reports backpressure so callers can preserve durable
// retry semantics. Dynamic topics are intended for configured integrations;
// core events should continue to use their generated, typed publishers.
func (bus *EventBus) Publish(event Event, payload any) error {
	select {
	case bus.ch <- envelope{event: event, payload: payload}:
		bus.runOnPublish(event, payload)
		return nil
	default:
		bus.runOnDrop(event, payload)
		return ErrFull
	}
}

// Subscribe registers a handler for a dynamically named in-process event.
func (bus *EventBus) Subscribe(event Event, fn func(any)) {
	bus.mu.Lock()
	bus.subscribers[event] = append(bus.subscribers[event], fn)
	bus.mu.Unlock()
	bus.runOnSubscribe(event)
}
