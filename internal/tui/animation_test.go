package tui

import (
	"testing"
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
)

func TestAnimationStore_RecordActivity(t *testing.T) {
	store := NewAnimationStore(4)

	// Record a publish activity
	store.RecordActivity(messaging.Activity{
		ID:        "1",
		Type:      messaging.ActivityPublish,
		Topic:     "test-topic",
		SessionID: "session-1",
		Timestamp: time.Now(),
	})

	anim := store.Get("session-1")
	if anim == nil {
		t.Fatal("expected animation for session-1")
	}
	if anim.Direction != AnimationSend {
		t.Errorf("expected AnimationSend, got %v", anim.Direction)
	}
	if anim.Topic != "test-topic" {
		t.Errorf("expected topic 'test-topic', got %q", anim.Topic)
	}
	if anim.TicksLeft != 4 {
		t.Errorf("expected 4 ticks, got %d", anim.TicksLeft)
	}
}

func TestAnimationStore_RecordSubscribe(t *testing.T) {
	store := NewAnimationStore(3)

	store.RecordActivity(messaging.Activity{
		ID:        "2",
		Type:      messaging.ActivitySubscribe,
		Topic:     "inbox.agent",
		SessionID: "session-2",
		Timestamp: time.Now(),
	})

	anim := store.Get("session-2")
	if anim == nil {
		t.Fatal("expected animation for session-2")
	}
	if anim.Direction != AnimationRecv {
		t.Errorf("expected AnimationRecv, got %v", anim.Direction)
	}
}

func TestAnimationStore_Tick(t *testing.T) {
	store := NewAnimationStore(2)

	store.RecordActivity(messaging.Activity{
		ID:        "1",
		Type:      messaging.ActivityPublish,
		Topic:     "topic",
		SessionID: "session-1",
		Timestamp: time.Now(),
	})

	// First tick: 2 -> 1
	changed := store.Tick()
	if !changed {
		t.Error("expected changed=true after first tick")
	}
	anim := store.Get("session-1")
	if anim == nil || anim.TicksLeft != 1 {
		t.Errorf("expected 1 tick left, got %v", anim)
	}

	// Second tick: 1 -> 0 (removed)
	changed = store.Tick()
	if !changed {
		t.Error("expected changed=true after second tick")
	}
	anim = store.Get("session-1")
	if anim != nil {
		t.Errorf("expected nil animation after expiry, got %v", anim)
	}

	// Third tick: nothing to do
	changed = store.Tick()
	if changed {
		t.Error("expected changed=false when no animations")
	}
}

func TestAnimationStore_ReplaceAnimation(t *testing.T) {
	store := NewAnimationStore(4)

	// Record initial publish
	store.RecordActivity(messaging.Activity{
		ID:        "1",
		Type:      messaging.ActivityPublish,
		Topic:     "topic-a",
		SessionID: "session-1",
		Timestamp: time.Now(),
	})

	// Tick down
	store.Tick()
	store.Tick()
	anim := store.Get("session-1")
	if anim.TicksLeft != 2 {
		t.Errorf("expected 2 ticks, got %d", anim.TicksLeft)
	}

	// New activity replaces with fresh ticks
	store.RecordActivity(messaging.Activity{
		ID:        "2",
		Type:      messaging.ActivitySubscribe,
		Topic:     "topic-b",
		SessionID: "session-1",
		Timestamp: time.Now(),
	})

	anim = store.Get("session-1")
	if anim.TicksLeft != 4 {
		t.Errorf("expected 4 ticks after replacement, got %d", anim.TicksLeft)
	}
	if anim.Direction != AnimationRecv {
		t.Errorf("expected AnimationRecv after replacement, got %v", anim.Direction)
	}
	if anim.Topic != "topic-b" {
		t.Errorf("expected topic-b after replacement, got %q", anim.Topic)
	}
}

func TestAnimationStore_IgnoreEmptySessionID(t *testing.T) {
	store := NewAnimationStore(4)

	// Activity without session ID should be ignored
	store.RecordActivity(messaging.Activity{
		ID:        "1",
		Type:      messaging.ActivityPublish,
		Topic:     "topic",
		SessionID: "",
		Timestamp: time.Now(),
	})

	anim := store.Get("")
	if anim != nil {
		t.Errorf("expected nil for empty session ID, got %v", anim)
	}
}

func TestAnimationStore_Clear(t *testing.T) {
	store := NewAnimationStore(4)

	store.RecordActivity(messaging.Activity{
		ID:        "1",
		Type:      messaging.ActivityPublish,
		Topic:     "topic",
		SessionID: "session-1",
		Timestamp: time.Now(),
	})
	store.RecordActivity(messaging.Activity{
		ID:        "2",
		Type:      messaging.ActivitySubscribe,
		Topic:     "topic",
		SessionID: "session-2",
		Timestamp: time.Now(),
	})

	store.Clear()

	if store.Get("session-1") != nil || store.Get("session-2") != nil {
		t.Error("expected all animations cleared")
	}
}
