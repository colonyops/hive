package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, time.September, 4, hour, minute, 0, 0, time.UTC)
}

func cursorAt(cron string, t time.Time) *Cursor {
	return &Cursor{Workspace: "product", ID: "weekly", EvaluatedThrough: t, Cron: cron}
}

func TestEvaluate(t *testing.T) {
	t.Parallel()

	const grace = 5 * time.Minute
	hourly := Spec{Workspace: "product", ID: "weekly", Cron: "0 * * * *", Prompt: "go"}

	tests := []struct {
		name    string
		spec    Spec
		cursor  *Cursor
		now     time.Time
		want    *Decision
		wantErr string
	}{
		{
			name:   "first sight never back-fires",
			spec:   hourly,
			cursor: nil,
			now:    at(9, 30),
		},
		{
			name:   "a changed cron never back-fires",
			spec:   hourly,
			cursor: cursorAt("0 9 * * 5", at(4, 0)),
			now:    at(9, 30),
		},
		{
			name:   "disabled",
			spec:   func() Spec { s := hourly; s.Disabled = true; return s }(),
			cursor: cursorAt(hourly.Cron, at(4, 0)),
			now:    at(9, 30),
		},
		{
			name:   "nothing due yet",
			spec:   hourly,
			cursor: cursorAt(hourly.Cron, at(9, 5)),
			now:    at(9, 30),
		},
		{
			// The occurrence at the cursor's own instant was evaluated by the
			// pass that closed the window there; counting it again would re-fire
			// the run that just happened.
			name:   "an occurrence exactly at the cursor is not due again",
			spec:   hourly,
			cursor: cursorAt(hourly.Cron, at(9, 0)),
			now:    at(9, 2),
		},
		{
			name:   "on time, inside the grace window",
			spec:   hourly,
			cursor: cursorAt(hourly.Cron, at(8, 30)),
			now:    at(9, 2),
			want:   &Decision{ScheduledFor: at(9, 0), Reason: ReasonDue, Missed: 0},
		},
		{
			name:   "exactly at the grace boundary is still due",
			spec:   hourly,
			cursor: cursorAt(hourly.Cron, at(8, 30)),
			now:    at(9, 5),
			want:   &Decision{ScheduledFor: at(9, 0), Reason: ReasonDue, Missed: 0},
		},
		{
			name:   "one occurrence, past the grace window",
			spec:   hourly,
			cursor: cursorAt(hourly.Cron, at(8, 30)),
			now:    at(9, 40),
			want:   &Decision{ScheduledFor: at(9, 0), Reason: ReasonCatchUp, Missed: 0},
		},
		{
			name:   "several missed occurrences collapse into the latest",
			spec:   hourly,
			cursor: cursorAt(hourly.Cron, at(5, 30)),
			now:    at(9, 40),
			want:   &Decision{ScheduledFor: at(9, 0), Reason: ReasonCatchUp, Missed: 3},
		},
		{
			name:   "the newest of several is still due when it is fresh",
			spec:   hourly,
			cursor: cursorAt(hourly.Cron, at(5, 30)),
			now:    at(9, 1),
			want:   &Decision{ScheduledFor: at(9, 0), Reason: ReasonDue, Missed: 3},
		},
		{
			name:   "on_missed skip marks a late run to skip",
			spec:   func() Spec { s := hourly; s.OnMissed = OnMissedSkip; return s }(),
			cursor: cursorAt(hourly.Cron, at(5, 30)),
			now:    at(9, 40),
			want:   &Decision{ScheduledFor: at(9, 0), Reason: ReasonCatchUp, Missed: 3, Skip: true},
		},
		{
			name:   "on_missed skip does not skip an on-time run",
			spec:   func() Spec { s := hourly; s.OnMissed = OnMissedSkip; return s }(),
			cursor: cursorAt(hourly.Cron, at(8, 30)),
			now:    at(9, 1),
			want:   &Decision{ScheduledFor: at(9, 0), Reason: ReasonDue, Missed: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := Evaluate(tt.spec, tt.cursor, tt.now, grace)

			assert.Equal(t, Cursor{
				Workspace: tt.spec.Workspace, ID: tt.spec.ID,
				EvaluatedThrough: tt.now, Cron: tt.spec.Cron,
			}, got.Cursor, "the window always closes at now")

			if tt.want == nil {
				assert.Nil(t, got.Decision)
				return
			}
			require.NotNil(t, got.Decision)
			assert.Equal(t, *tt.want, *got.Decision)
		})
	}
}

// TestEvaluateCapsTheOccurrenceWalk covers the clock-jump case: a
// minute-by-minute schedule whose cursor is a year old would otherwise walk
// half a million occurrences before deciding to fire once.
func TestEvaluateCapsTheOccurrenceWalk(t *testing.T) {
	t.Parallel()

	spec := Spec{Workspace: "product", ID: "minutely", Cron: "* * * * *", Prompt: "go"}
	start := time.Date(2025, time.September, 4, 0, 0, 0, 0, time.UTC)
	now := start.AddDate(1, 0, 0)

	got := Evaluate(spec, &Cursor{Workspace: spec.Workspace, ID: spec.ID, EvaluatedThrough: start, Cron: spec.Cron}, now, time.Minute)

	require.NotNil(t, got.Decision)
	assert.Equal(t, maxOccurrences-1, got.Decision.Missed)
	assert.Equal(t, start.Add(maxOccurrences*time.Minute), got.Decision.ScheduledFor)
	assert.Equal(t, ReasonCatchUp, got.Decision.Reason)
	assert.Equal(t, now, got.Cursor.EvaluatedThrough, "the window still closes at now, so the next pass starts fresh")
}

func TestNextDue(t *testing.T) {
	t.Parallel()

	now := at(9, 30)

	tests := []struct {
		name  string
		specs []Spec
		want  time.Time
		ok    bool
	}{
		{name: "no specs", specs: nil},
		{
			name:  "one spec",
			specs: []Spec{{ID: "a", Cron: "0 * * * *", Prompt: "go"}},
			want:  at(10, 0),
			ok:    true,
		},
		{
			name: "the soonest of several",
			specs: []Spec{
				{ID: "a", Cron: "0 * * * *", Prompt: "go"},
				{ID: "b", Cron: "45 * * * *", Prompt: "go"},
			},
			want: at(9, 45),
			ok:   true,
		},
		{
			name: "disabled specs do not count",
			specs: []Spec{
				{ID: "a", Cron: "0 * * * *", Prompt: "go"},
				{ID: "b", Cron: "45 * * * *", Prompt: "go", Disabled: true},
			},
			want: at(10, 0),
			ok:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := NextDue(tt.specs, now)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
