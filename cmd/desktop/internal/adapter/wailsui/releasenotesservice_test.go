package wailsui

import (
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/releasenotes"
)

func newTestCore(t *testing.T) *app.ReleaseNotesService {
	t.Helper()
	return app.NewReleaseNotesService(settings.Paths{StateDir: t.TempDir()}, zerolog.Nop())
}

// A source build has no release to describe, so nothing surfaces on launch —
// but the changelog it embeds is still readable in About.
func TestReleaseNotesServiceIsSilentOnSourceBuilds(t *testing.T) {
	service := NewReleaseNotesService(newTestCore(t), "dev")

	assert.False(t, service.Pending(t.Context()).Show)
	assert.NotEmpty(t, service.History(t.Context()), "a source build can still read the changelog")
}

// The draft crosses the boundary with neither a version nor a date: it
// describes no release until it is promoted into one, and a zero time
// formatted as a date would render as the year 1.
func TestReleaseNotesServiceSendsTheDraftWithoutAVersionOrDate(t *testing.T) {
	notes := releaseNotes(releasenotes.Entries{{Draft: true, Summary: "in progress", Body: "unreleased work"}})

	require.Len(t, notes, 1)
	assert.True(t, notes[0].Draft)
	assert.Empty(t, notes[0].Version)
	assert.Empty(t, notes[0].Date)
	assert.Equal(t, "in progress", notes[0].Summary)
}

func TestReleaseNotesServiceMapsEntriesForTheFrontend(t *testing.T) {
	notes := releaseNotes(testReleaseEntries(t))

	require.NotEmpty(t, notes)
	assert.Equal(t, "1.2.0", notes[0].Version)
	assert.NotEmpty(t, notes[0].Body)
	// Dates cross the boundary as plain YYYY-MM-DD, not as an instant.
	assert.Equal(t, "2026-01-02", notes[0].Date)
}

func testReleaseEntries(t *testing.T) releasenotes.Entries {
	t.Helper()
	date, err := time.Parse(time.DateOnly, "2026-01-02")
	require.NoError(t, err)
	return releasenotes.Entries{{Version: "1.2.0", Date: date, Body: "notes for 1.2.0"}}
}

func TestReleaseNotesServicePendingAndAcknowledge(t *testing.T) {
	core := newTestCore(t)
	require.NoError(t, core.Acknowledge(t.Context(), "0.0.1"))

	service := NewReleaseNotesService(core, "1.2.0-dev.9")
	require.True(t, service.Pending(t.Context()).Show, "a build newer than the acknowledged version surfaces")

	require.NoError(t, service.Acknowledge(t.Context()))
	assert.False(t, service.Pending(t.Context()).Show)
}
