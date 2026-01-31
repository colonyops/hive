package tui

import (
	"time"

	"github.com/hay-kot/hive/internal/core/messaging"
)

// AnimationDirection represents the direction of a message animation.
type AnimationDirection int

const (
	AnimationNone AnimationDirection = iota
	AnimationSend                    // Session is publishing
	AnimationRecv                    // Session is receiving/subscribing
)

// SessionAnimation represents an active animation for a session.
type SessionAnimation struct {
	Direction AnimationDirection
	Topic     string
	TicksLeft int
	StartedAt time.Time
}

// AnimationStore tracks animations for sessions.
type AnimationStore struct {
	animations map[string]SessionAnimation // session ID -> animation
	ticksMax   int                         // default ticks for new animations
}

// NewAnimationStore creates a new animation store.
func NewAnimationStore(ticksMax int) *AnimationStore {
	return &AnimationStore{
		animations: make(map[string]SessionAnimation),
		ticksMax:   ticksMax,
	}
}

// Get returns the animation for a session, or nil if none.
func (s *AnimationStore) Get(sessionID string) *SessionAnimation {
	anim, ok := s.animations[sessionID]
	if !ok || anim.Direction == AnimationNone {
		return nil
	}
	return &anim
}

// RecordActivity updates animations based on an activity event.
func (s *AnimationStore) RecordActivity(activity messaging.Activity) {
	dir := AnimationNone
	switch activity.Type {
	case messaging.ActivityPublish:
		dir = AnimationSend
	case messaging.ActivitySubscribe:
		dir = AnimationRecv
	}

	if dir == AnimationNone || activity.SessionID == "" {
		return
	}

	s.animations[activity.SessionID] = SessionAnimation{
		Direction: dir,
		Topic:     activity.Topic,
		TicksLeft: s.ticksMax,
		StartedAt: time.Now(),
	}
}

// Tick decrements all animation tick counts and removes expired ones.
// Returns true if any animations were updated (for triggering rerender).
func (s *AnimationStore) Tick() bool {
	changed := false
	for sessionID, anim := range s.animations {
		if anim.TicksLeft > 0 {
			anim.TicksLeft--
			if anim.TicksLeft <= 0 {
				delete(s.animations, sessionID)
			} else {
				s.animations[sessionID] = anim
			}
			changed = true
		}
	}
	return changed
}

// Clear removes all animations.
func (s *AnimationStore) Clear() {
	s.animations = make(map[string]SessionAnimation)
}
