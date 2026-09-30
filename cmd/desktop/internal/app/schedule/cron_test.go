package schedule

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		expr string
		ok   bool
	}{
		{"five fields", "0 9 * * 5", true},
		{"leading and trailing space", "  0 9 * * 5  ", true},
		{"daily descriptor", "@daily", true},
		{"empty", "", false},
		{"only spaces", "   ", false},
		{"six fields is not standard cron", "0 0 9 * * 5", false},
		{"gibberish", "not a cron", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sched, err := ParseCron(tt.expr)
			if !tt.ok {
				require.Error(t, err)
				assert.Nil(t, sched)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, sched)
		})
	}
}

func TestNextOccurrences(t *testing.T) {
	t.Parallel()

	// A Wednesday, so "every Friday at 09:00" has two clear days to run.
	after := time.Date(2026, time.September, 2, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		expr string
		n    int
		want []time.Time
	}{
		{
			name: "weekly",
			expr: "0 9 * * 5",
			n:    2,
			want: []time.Time{
				time.Date(2026, time.September, 4, 9, 0, 0, 0, time.UTC),
				time.Date(2026, time.September, 11, 9, 0, 0, 0, time.UTC),
			},
		},
		{
			name: "zero count",
			expr: "@hourly",
			n:    0,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NextOccurrences(tt.expr, after, tt.n)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
