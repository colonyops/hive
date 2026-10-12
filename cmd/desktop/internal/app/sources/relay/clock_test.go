package relay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/relayformat"
	"github.com/colonyops/hive/pkg/duration"
)

var clockStart = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

// ping merges a healthchecks delivery for /backup<suffix> received at the
// given offset from clockStart.
func ping(t *testing.T, n *nodeState, suffix string, at time.Duration) {
	t.Helper()
	d := relayformat.Delivery{ID: suffix + at.String(), ReceivedAt: clockStart.Add(at), Method: "GET", Path: "/backup" + suffix}
	items, _, err := relayformat.Parse(relayformat.DialectHealthchecks, d)
	require.NoError(t, err)
	n.merge(items[0].Key, item{payload: items[0].Payload, receivedAt: d.ReceivedAt})
}

func TestApplyClock(t *testing.T) {
	t.Parallel()

	checks := map[string]Check{"backup": {Period: duration.Duration(time.Hour), Grace: duration.Duration(10 * time.Minute)}}

	type step struct {
		ping      string
		pingAt    time.Duration
		clockAt   time.Duration
		wantState any
		wantBody  any
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{name: "fresh", steps: []step{
			{ping: "", pingAt: 0, clockAt: 30 * time.Minute, wantState: "resolved", wantBody: nil},
		}},
		{name: "grace respected", steps: []step{
			{ping: "", pingAt: 0, clockAt: 70 * time.Minute, wantState: "resolved", wantBody: nil},
		}},
		{name: "overdue once", steps: []step{
			{ping: "", pingAt: 0, clockAt: 71 * time.Minute, wantState: "down", wantBody: "no ping for 1h11m0s"},
		}},
		{name: "overdue twice keeps the first down", steps: []step{
			{ping: "", pingAt: 0, clockAt: 71 * time.Minute, wantState: "down", wantBody: "no ping for 1h11m0s"},
			{ping: "-", clockAt: 3 * time.Hour, wantState: "down", wantBody: "no ping for 1h11m0s"},
		}},
		{name: "recovered", steps: []step{
			{ping: "", pingAt: 0, clockAt: 2 * time.Hour, wantState: "down", wantBody: "no ping for 2h0m0s"},
			{ping: "", pingAt: 2*time.Hour + time.Minute, clockAt: 2*time.Hour + 2*time.Minute, wantState: "resolved", wantBody: nil},
		}},
		{name: "start does not reset the clock or the state", steps: []step{
			{ping: "", pingAt: 0, clockAt: 30 * time.Minute, wantState: "resolved"},
			{ping: "/start", pingAt: 60 * time.Minute, clockAt: 71 * time.Minute, wantState: "down", wantBody: "no ping for 1h11m0s"},
		}},
		{name: "start alone counts from the start", steps: []step{
			{ping: "/start", pingAt: 0, clockAt: 30 * time.Minute, wantState: nil},
			{ping: "-", clockAt: 71 * time.Minute, wantState: "down", wantBody: "no ping for 1h11m0s"},
		}},
		{name: "a failed check stays as the parser left it", steps: []step{
			{ping: "/fail", pingAt: 0, clockAt: 3 * time.Hour, wantState: "down", wantBody: nil},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := &nodeState{items: map[string]item{}}
			var previous map[string]any
			for i, s := range tc.steps {
				if s.ping != "-" {
					ping(t, n, s.ping, s.pingAt)
				}
				n.applyClock(checks, clockStart.Add(s.clockAt))
				got := n.items["backup"]
				assert.Equal(t, s.wantState, got.payload["state"], "step %d state", i)
				assert.Equal(t, s.wantBody, got.payload["body"], "step %d body", i)
				if s.ping == "-" && previous != nil && previous["state"] == "down" {
					assert.Equal(t, previous, got.payload, "step %d: a down check must not churn", i)
				}
				previous = got.payload
			}
		})
	}
}

func TestApplyClockSkipsASlugWithNoItem(t *testing.T) {
	t.Parallel()
	n := &nodeState{items: map[string]item{}}
	n.applyClock(map[string]Check{"backup": {Period: duration.Duration(time.Minute)}}, clockStart.Add(time.Hour))
	assert.Empty(t, n.items)
	assert.Empty(t, n.order)
}

func TestMergeKeepsStateForStartAndLog(t *testing.T) {
	t.Parallel()
	n := &nodeState{items: map[string]item{}}
	ping(t, n, "/fail", 0)
	ping(t, n, "/log", time.Minute)
	got := n.items["backup"].payload
	assert.Equal(t, "down", got["state"])
	assert.Equal(t, "log", got["ping"])
	assert.Equal(t, clockStart.Add(time.Minute).Format(time.RFC3339Nano), got["pinged_at"])

	ping(t, n, "", 2*time.Minute)
	assert.Equal(t, "resolved", n.items["backup"].payload["state"])
	assert.Equal(t, []string{"backup"}, n.order)
}
