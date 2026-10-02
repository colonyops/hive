package app

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/releasenotes"
)

func newTestReleaseNotes(t *testing.T, entries releasenotes.Entries) *ReleaseNotesService {
	t.Helper()
	return &ReleaseNotesService{entries: entries, state: releasenotes.NewState(t.TempDir())}
}

func testEntries(versions ...string) releasenotes.Entries {
	out := make(releasenotes.Entries, 0, len(versions))
	for _, v := range versions {
		out = append(out, releasenotes.Entry{Version: v, Body: "notes for " + v})
	}
	return out
}

func testDraft() releasenotes.Entry {
	return releasenotes.Entry{Draft: true, Summary: "in progress", Body: "unreleased work"}
}

// A fresh install has no version it upgraded *from*, so it adopts the running
// version silently rather than announcing a release the user never crossed.
func TestPendingIsSilentOnAFreshInstall(t *testing.T) {
	service := newTestReleaseNotes(t, testEntries("1.2.0"))

	assert.Equal(t, PendingNotes{}, service.Pending(t.Context(), "1.2.0"))
	assert.Equal(t, PendingNotes{}, service.Pending(t.Context(), "1.2.0"),
		"the recorded version keeps it silent on the next launch too")
}

func TestPendingShowsNotesAfterAnUpgrade(t *testing.T) {
	service := newTestReleaseNotes(t, testEntries("1.3.0", "1.2.0"))
	require.NoError(t, service.Acknowledge(t.Context(), "1.2.0"))

	pending := service.Pending(t.Context(), "1.3.0")
	require.True(t, pending.Show)
	assert.Equal(t, PresentationModal, pending.Presentation)
	assert.Equal(t, "1.3.0", pending.Version)
	require.Len(t, pending.Entries, 1)
	assert.Equal(t, "1.3.0", pending.Entries[0].Version)
}

// The acceptance criterion: shown once, then not again for that version.
func TestPendingStopsAfterAcknowledgement(t *testing.T) {
	service := newTestReleaseNotes(t, testEntries("1.3.0", "1.2.0"))
	require.NoError(t, service.Acknowledge(t.Context(), "1.2.0"))

	require.True(t, service.Pending(t.Context(), "1.3.0").Show)
	require.True(t, service.Pending(t.Context(), "1.3.0").Show,
		"an unacknowledged surface returns until it is dismissed")

	require.NoError(t, service.Acknowledge(t.Context(), "1.3.0"))
	assert.Equal(t, PendingNotes{}, service.Pending(t.Context(), "1.3.0"))
}

func TestPendingIsSilentOnSourceBuilds(t *testing.T) {
	service := newTestReleaseNotes(t, testEntries("1.3.0"))
	require.NoError(t, service.Acknowledge(t.Context(), "1.2.0"))

	assert.Equal(t, PendingNotes{}, service.Pending(t.Context(), "dev"))
	assert.Equal(t, PendingNotes{}, service.Pending(t.Context(), "(devel)"))
}

func TestPendingIsSilentOnADowngrade(t *testing.T) {
	service := newTestReleaseNotes(t, testEntries("1.3.0", "1.2.0"))
	require.NoError(t, service.Acknowledge(t.Context(), "1.3.0"))

	assert.Equal(t, PendingNotes{}, service.Pending(t.Context(), "1.2.0"))
}

// A bump that carries only the draft — an install from before the shared
// version moving between prereleases — shows the same in-progress list the
// previous build showed, so it reports the bump in a toast rather than
// reopening a modal over it.
func TestPendingUsesAToastWhenOnlyTheDraftIsNew(t *testing.T) {
	service := newTestReleaseNotes(t, releasenotes.Entries{testDraft()})
	require.NoError(t, service.Acknowledge(t.Context(), "1.2.0-dev.3"))

	pending := service.Pending(t.Context(), "1.2.0-dev.4")
	require.True(t, pending.Show)
	assert.Equal(t, PresentationToast, pending.Presentation)
	require.Len(t, pending.Entries, 1)
	assert.True(t, pending.Entries[0].Draft, "the draft is still what the toast has to offer")
}

// Crossing a release is the one launch worth a modal, whichever channel the
// build itself follows — an install still on a prerelease from before the
// shared version has arrived at the release it was testing.
func TestPendingUsesAModalWhenAReleaseIsCrossed(t *testing.T) {
	service := newTestReleaseNotes(t, releasenotes.Entries{testDraft(), releasenotes.Entry{Version: "1.2.0", Body: "notes"}})
	require.NoError(t, service.Acknowledge(t.Context(), "1.2.0-dev.9"))

	pending := service.Pending(t.Context(), "1.2.0")
	require.True(t, pending.Show)
	assert.Equal(t, PresentationModal, pending.Presentation)
}

// An upgrade with nothing in range still reports the version bump — a modal
// with an empty body would say less than the one-line toast does.
func TestPendingDegradesToAToastWhenNoEntriesExist(t *testing.T) {
	service := newTestReleaseNotes(t, testEntries("1.2.0"))
	require.NoError(t, service.Acknowledge(t.Context(), "1.2.0"))

	pending := service.Pending(t.Context(), "1.4.0")
	require.True(t, pending.Show)
	assert.Equal(t, PresentationToast, pending.Presentation)
	assert.Empty(t, pending.Entries)
	assert.Equal(t, "1.4.0", pending.Version)
}

func TestPendingCollectsEverySkippedRelease(t *testing.T) {
	service := newTestReleaseNotes(t, testEntries("1.4.0", "1.3.0", "1.2.0"))
	require.NoError(t, service.Acknowledge(t.Context(), "1.2.0"))

	pending := service.Pending(t.Context(), "1.4.0")
	require.Len(t, pending.Entries, 2, "both crossed releases are reported")
	assert.Equal(t, "1.4.0", pending.Entries[0].Version)
	assert.Equal(t, "1.3.0", pending.Entries[1].Version)
}

// History is not scoped to a channel: every entry is either a release, which
// every channel manifest received, or this build's own draft.
func TestHistoryIsEverythingTheBuildCarries(t *testing.T) {
	entries := releasenotes.Entries{testDraft(), releasenotes.Entry{Version: "1.2.0", Body: "notes"}}
	service := newTestReleaseNotes(t, entries)

	assert.Equal(t, entries, service.History(t.Context()))
}

// The constructor degrades rather than failing when the changelog cannot be
// parsed, because release notes are not worth refusing to launch over.
func TestNewReleaseNotesServiceLoadsTheEmbeddedChangelog(t *testing.T) {
	service := NewReleaseNotesService(settings.Paths{StateDir: t.TempDir()}, zerolog.Nop())

	assert.NotEmpty(t, service.History(t.Context()))
}
