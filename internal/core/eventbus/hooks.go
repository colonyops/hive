package eventbus

import "sync"

// hooks holds the lifecycle hook state for the EventBus.
// These are separated from the generated code since the generator
// produces only typed Publish/Subscribe pairs.
type hooks struct {
	mu          sync.RWMutex
	onPublish   []func(Event, any)
	onDrop      []func(Event, any)
	onSubscribe []func(Event)
	onPanic     []func(Event, any, any)
}

// OnPublish registers a hook that fires after an event is successfully enqueued.
func (bus *EventBus) OnPublish(fn func(Event, any)) {
	bus.hooks.mu.Lock()
	bus.hooks.onPublish = append(bus.hooks.onPublish, fn)
	bus.hooks.mu.Unlock()
}

// OnDrop registers a hook that fires when an event is dropped due to a full buffer.
func (bus *EventBus) OnDrop(fn func(Event, any)) {
	bus.hooks.mu.Lock()
	bus.hooks.onDrop = append(bus.hooks.onDrop, fn)
	bus.hooks.mu.Unlock()
}

// OnSubscribe registers a hook that fires after a subscriber is registered.
func (bus *EventBus) OnSubscribe(fn func(Event)) {
	bus.hooks.mu.Lock()
	bus.hooks.onSubscribe = append(bus.hooks.onSubscribe, fn)
	bus.hooks.mu.Unlock()
}

// OnPanic registers a hook that fires when a subscriber panics.
func (bus *EventBus) OnPanic(fn func(Event, any, any)) {
	bus.hooks.mu.Lock()
	bus.hooks.onPanic = append(bus.hooks.onPanic, fn)
	bus.hooks.mu.Unlock()
}

// send enqueues an event and fires hooks. Used by generated Publish* methods.
func (bus *EventBus) send(event Event, payload any) {
	select {
	case bus.ch <- envelope{event: event, payload: payload}:
		bus.runOnPublish(event, payload)
	default:
		bus.runOnDrop(event, payload)
	}
}

func (bus *EventBus) runOnPublish(event Event, payload any) {
	bus.hooks.mu.RLock()
	hooks := make([]func(Event, any), len(bus.hooks.onPublish))
	copy(hooks, bus.hooks.onPublish)
	bus.hooks.mu.RUnlock()
	for _, fn := range hooks {
		fn(event, payload)
	}
}

func (bus *EventBus) runOnDrop(event Event, payload any) {
	bus.hooks.mu.RLock()
	hooks := make([]func(Event, any), len(bus.hooks.onDrop))
	copy(hooks, bus.hooks.onDrop)
	bus.hooks.mu.RUnlock()
	for _, fn := range hooks {
		fn(event, payload)
	}
}

func (bus *EventBus) runOnPanic(event Event, payload any, recovered any) {
	bus.hooks.mu.RLock()
	hooks := make([]func(Event, any, any), len(bus.hooks.onPanic))
	copy(hooks, bus.hooks.onPanic)
	bus.hooks.mu.RUnlock()
	for _, fn := range hooks {
		func() {
			defer func() { recover() }() //nolint:errcheck
			fn(event, payload, recovered)
		}()
	}
}
