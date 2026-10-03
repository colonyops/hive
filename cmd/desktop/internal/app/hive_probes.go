package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/hivewatch"
	"github.com/colonyops/hive/internal/domain/hc"
	"github.com/colonyops/hive/internal/domain/session"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
)

// hiveWatchInterval bounds how long a CLI write to hive.db takes to reach the
// UI. A filesystem watch cannot replace it: every table shares that one file
// and its WAL, so a watch fires for message and status writes too.
const hiveWatchInterval = 2 * time.Second

// sessionSignature is the part of a session the session list shows. A change
// to anything else, such as UpdatedAt, is not a change the UI re-reads for.
type sessionSignature struct {
	ID          string
	Name        string
	Slug        string
	Remote      string
	State       session.State
	TmuxSession string
}

func signatureOf(s session.Session) sessionSignature {
	return sessionSignature{
		ID:          s.ID,
		Name:        s.Name,
		Slug:        s.Slug,
		Remote:      s.Remote,
		State:       s.State,
		TmuxSession: sessionsvc.Target(s).Session,
	}
}

func sessionsProbe(list func(context.Context) ([]session.Session, error), bus *events.Bus) hivewatch.Probe {
	return hivewatch.NewProbe(hivewatch.Spec[[]sessionSignature]{
		Name: "sessions",
		Read: func(ctx context.Context) ([]sessionSignature, error) {
			sessions, err := list(ctx)
			if err != nil {
				return nil, err
			}
			signatures := make([]sessionSignature, 0, len(sessions))
			for _, s := range sessions {
				signatures = append(signatures, signatureOf(s))
			}
			slices.SortFunc(signatures, func(a, b sessionSignature) int { return strings.Compare(a.ID, b.ID) })
			return signatures, nil
		},
		Equal: slices.Equal[[]sessionSignature],
		OnChange: func(ctx context.Context, prev, next []sessionSignature) {
			added, changed, removed := diffSessions(prev, next)
			bus.Publish(ctx, events.SessionsUpdated{Added: added, Changed: changed, Removed: removed})
		},
	})
}

func tasksProbe(fingerprint func(context.Context) (hc.Fingerprint, error), bus *events.Bus) hivewatch.Probe {
	return hivewatch.NewProbe(hivewatch.Spec[hc.Fingerprint]{
		Name:  "tasks",
		Read:  fingerprint,
		Equal: func(a, b hc.Fingerprint) bool { return a == b },
		OnChange: func(ctx context.Context, _, _ hc.Fingerprint) {
			bus.Publish(ctx, events.TasksUpdated{})
		},
	})
}

// diffSessions takes two id-sorted sets and returns sorted ids.
func diffSessions(prev, next []sessionSignature) (added, changed, removed []string) {
	before := make(map[string]sessionSignature, len(prev))
	for _, s := range prev {
		before[s.ID] = s
	}
	for _, s := range next {
		was, ok := before[s.ID]
		switch {
		case !ok:
			added = append(added, s.ID)
		case was != s:
			changed = append(changed, s.ID)
		}
		delete(before, s.ID)
	}
	for _, s := range prev {
		if _, gone := before[s.ID]; gone {
			removed = append(removed, s.ID)
		}
	}
	return added, changed, removed
}
