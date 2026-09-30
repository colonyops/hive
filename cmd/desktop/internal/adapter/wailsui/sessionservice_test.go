package wailsui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app/dispatch"
)

func TestSessionStatusSnapshotOfConvertsPollIntervalForBrowser(t *testing.T) {
	items := []dispatch.SessionStatus{{
		SessionID: "s1",
		Running:   true,
		Windows:   []dispatch.SessionWindowStatus{{WindowID: "@1", Status: "approval", Tool: "claude"}},
	}}

	got := sessionStatusSnapshotOf(dispatch.SessionStatusSnapshot{
		Items:        items,
		PollInterval: 1750 * time.Millisecond,
	})

	assert.Equal(t, items, got.Items)
	assert.Equal(t, int64(1750), got.PollIntervalMS)
}
