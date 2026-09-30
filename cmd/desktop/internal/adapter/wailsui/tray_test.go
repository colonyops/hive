package wailsui

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

func TestTrayProfilesIncludesValidAndInvalidFlows(t *testing.T) {
	summaries := []FlowSummary{
		{ID: "broken", Valid: false},
		{ID: "triage-id", Name: "Triage", Valid: true},
	}

	assert.Equal(t, []trayProfile{
		{ID: "broken", Label: "broken (invalid)"},
		{ID: "triage-id", Label: "Triage", Valid: true},
	}, trayProfiles(summaries))
}

func TestTrayUpdated(t *testing.T) {
	now := time.Date(2026, 9, 24, 16, 0, 0, 0, time.UTC)
	assert.Equal(t, "Not polled yet", trayUpdated(time.Time{}, now))
	assert.Equal(t, "Updated at 3:04 PM", trayUpdated(time.Date(2026, 9, 24, 15, 4, 0, 0, time.UTC), now))
	assert.Equal(t, "Updated Sep 23, 3:04 PM", trayUpdated(time.Date(2026, 9, 23, 15, 4, 0, 0, time.UTC), now))
}

func TestTrayFeedPath(t *testing.T) {
	assert.Equal(t, "Work › Deps › Renovate PRs", trayFeedPath(app.MenuBarFeedName{ProfileName: "Work", Folder: "Deps", Name: "Renovate PRs"}))
	assert.Equal(t, "Work › Open PRs", trayFeedPath(app.MenuBarFeedName{ProfileName: "Work", Name: "Open PRs"}))
}

func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, "Rotate credentials", truncateRunes("Rotate credentials", trayTitleLimit))
	assert.Equal(t, strings.Repeat("x", trayTitleLimit-1)+"…", truncateRunes(strings.Repeat("x", 70), trayTitleLimit))
}
