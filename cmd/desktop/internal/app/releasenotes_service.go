package app

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/cmd/desktop/releasenotes"
)

// How a pending set of release notes should be surfaced.
const (
	PresentationModal = "modal"
	PresentationToast = "toast"
)

// PendingNotes is the answer to "did this launch land on a newer version, and
// what should the user be told about it".
type PendingNotes struct {
	Show bool
	// Presentation is PresentationModal or PresentationToast; empty when
	// Show is false.
	Presentation string
	// Version is the running version the notes describe.
	Version string
	// Entries are the stable releases crossed to reach Version plus this
	// build's draft, newest first. It can be empty on a genuine upgrade when
	// nothing in the range carries notes.
	Entries releasenotes.Entries
}

// ReleaseNotesService decides what the What's New surface shows and remembers
// what the user has already seen. The running version is an argument rather
// than a field because the ldflags that carry the build identity bind to
// package main — the adapter supplies it, the same way SystemService
// receives it.
type ReleaseNotesService struct {
	entries releasenotes.Entries
	state   *releasenotes.State
}

// NewReleaseNotesService builds the service over the embedded changelog. It is
// exported for the same reason NewSettingsService is: it depends on nothing but
// a path and a logger, so the adapter can build one without standing up an App.
func NewReleaseNotesService(paths settings.Paths, logger zerolog.Logger) *ReleaseNotesService {
	entries, err := releasenotes.Load()
	if err != nil {
		// A malformed entry is a committed mistake that the package's own test
		// and the release gate both catch, so it cannot reach a published
		// build. If one somehow does, the app runs without release notes
		// rather than refusing to launch over them.
		logger.Warn().Err(err).Msg("release notes unavailable; changelog failed to parse")
	}
	return &ReleaseNotesService{entries: entries, state: releasenotes.NewState(paths.StateDir)}
}

// Pending reports what this launch should show. It is the launch-time half of
// the feature: an install ends by relaunching the app, so there is no
// in-process moment after an update to hook — the reliable trigger is finding,
// on the next start, that the running version outranks the last one the user
// acknowledged. That also covers updates this app did not perform, like a
// package manager or a manual reinstall.
//
// Recording on first run is a deliberate write from a read: a fresh install
// has no prior version to have upgraded *from*, so it silently adopts the
// running version and shows nothing.
func (s *ReleaseNotesService) Pending(_ context.Context, version string) PendingNotes {
	if !releasenotes.IsPublished(version) {
		return PendingNotes{}
	}

	acknowledged := s.state.Acknowledged()
	if acknowledged == "" {
		_ = s.state.Acknowledge(version)
		return PendingNotes{}
	}
	if !releasenotes.IsNewer(version, acknowledged) {
		return PendingNotes{}
	}

	entries := s.entries.Between(acknowledged, version)
	return PendingNotes{
		Show:         true,
		Presentation: presentationFor(entries),
		Version:      version,
		Entries:      entries,
	}
}

// presentationFor reserves the modal for a launch that crossed a stable
// release. A prerelease bump carries only the draft, which is the same
// in-progress list the previous one showed, and prereleases are cut close to
// daily — a modal on nearly every launch is the nuisance the toast exists to
// avoid. A range with nothing in it degrades to the toast for a different
// reason: a modal with an empty body says less than the line "Updated to X".
func presentationFor(entries releasenotes.Entries) string {
	for _, entry := range entries {
		if !entry.Draft {
			return PresentationModal
		}
	}
	return PresentationToast
}

// Acknowledge records version as seen. The modal defers this to its dismissal,
// so a crash while it is open does not cost the user the notes; the toast has
// nothing to dismiss and acknowledges on sight.
//
// The context is deliberately discarded: the marker write must finish whether
// or not the caller is still listening. A frontend component unmounting mid
// launch cancels its call, and a cancelled write here would resurface the
// notes on every subsequent launch.
func (s *ReleaseNotesService) Acknowledge(_ context.Context, version string) error {
	return s.state.Acknowledge(version)
}

// History is every set of notes this build carries, newest first: the draft
// for what it has that no stable release does, then the stable releases
// themselves. It is the About pane's list, and where a dismissed surface can
// be read again.
func (s *ReleaseNotesService) History(_ context.Context) releasenotes.Entries {
	return s.entries
}
