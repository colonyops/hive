package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/hivewatch"
)

// hiveWatchInterval bounds how long a CLI write to hive.db takes to reach the
// UI. A filesystem watch cannot replace it: every table shares that one file
// and its WAL, so a watch fires for message and status writes too.
const hiveWatchInterval = 2 * time.Second

type sessionLister interface {
	ListSessions(context.Context) ([]dispatch.SessionSummary, error)
}

type tasksFingerprinter interface {
	TasksFingerprint(context.Context) (dispatch.TasksFingerprint, error)
}

func sessionsProbe(lister sessionLister, bus *events.Bus) hivewatch.Probe {
	return hivewatch.NewProbe(hivewatch.Spec[[]dispatch.SessionSummary]{
		Name: "sessions",
		Read: func(ctx context.Context) ([]dispatch.SessionSummary, error) {
			sessions, err := lister.ListSessions(ctx)
			if err != nil {
				return nil, err
			}
			slices.SortFunc(sessions, func(a, b dispatch.SessionSummary) int { return strings.Compare(a.ID, b.ID) })
			return sessions, nil
		},
		Equal: slices.Equal[[]dispatch.SessionSummary],
		OnChange: func(ctx context.Context, prev, next []dispatch.SessionSummary) {
			added, changed, removed := diffSessions(prev, next)
			bus.Publish(ctx, events.SessionsUpdated{Added: added, Changed: changed, Removed: removed})
		},
	})
}

func tasksProbe(tasks tasksFingerprinter, bus *events.Bus) hivewatch.Probe {
	return hivewatch.NewProbe(hivewatch.Spec[dispatch.TasksFingerprint]{
		Name:  "tasks",
		Read:  tasks.TasksFingerprint,
		Equal: func(a, b dispatch.TasksFingerprint) bool { return a == b },
		OnChange: func(ctx context.Context, _, _ dispatch.TasksFingerprint) {
			bus.Publish(ctx, events.TasksUpdated{})
		},
	})
}

// diffSessions takes two id-sorted sets and returns sorted ids.
func diffSessions(prev, next []dispatch.SessionSummary) (added, changed, removed []string) {
	before := make(map[string]dispatch.SessionSummary, len(prev))
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
