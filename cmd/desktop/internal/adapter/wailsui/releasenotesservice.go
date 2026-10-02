package wailsui

import (
	"context"
	"time"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/releasenotes"
)

// ReleaseNote is the frontend-facing view of one set of release notes. Date is
// a plain YYYY-MM-DD string rather than a timestamp: a release is dated, not
// clocked, and formatting it here keeps the frontend from having to decide
// what an instant means in the user's timezone.
//
// Version and Date are empty when Draft is set — unreleased work has neither
// until it is promoted into a stable release.
type ReleaseNote struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
	Draft   bool   `json:"draft"`
}

// PendingReleaseNotes tells the frontend whether this launch landed on a newer
// version and how to say so.
type PendingReleaseNotes struct {
	Show bool `json:"show"`
	// Presentation is "modal" or "toast"; empty when Show is false.
	Presentation string        `json:"presentation"`
	Version      string        `json:"version"`
	Entries      []ReleaseNote `json:"entries"`
}

// ReleaseNotesService exposes the embedded changelog: what to show once after
// an update, and the full history the About pane lists.
//
// It takes no channel. Every release writes every channel manifest, so each
// entry a build carries is either a release every channel received or this
// build's own draft, and there is nothing a user on one channel must be kept
// from seeing.
type ReleaseNotesService struct {
	core    *app.ReleaseNotesService
	version string
}

func NewReleaseNotesService(core *app.ReleaseNotesService, version string) *ReleaseNotesService {
	return &ReleaseNotesService{core: core, version: version}
}

// Pending reports what this launch should show, if anything.
func (s *ReleaseNotesService) Pending(ctx context.Context) PendingReleaseNotes {
	pending := s.core.Pending(ctx, s.version)
	return PendingReleaseNotes{
		Show:         pending.Show,
		Presentation: pending.Presentation,
		Version:      pending.Version,
		Entries:      releaseNotes(pending.Entries),
	}
}

// Acknowledge records the running version as seen, so the surface does not
// return on the next launch.
func (s *ReleaseNotesService) Acknowledge(ctx context.Context) error {
	return s.core.Acknowledge(ctx, s.version)
}

// History lists this build's draft followed by every stable release it knows
// of, newest first.
func (s *ReleaseNotesService) History(ctx context.Context) []ReleaseNote {
	return releaseNotes(s.core.History(ctx))
}

func releaseNotes(entries releasenotes.Entries) []ReleaseNote {
	out := make([]ReleaseNote, 0, len(entries))
	for _, entry := range entries {
		note := ReleaseNote{Summary: entry.Summary, Body: entry.Body, Draft: entry.Draft}
		if !entry.Draft {
			note.Version = entry.Version
			note.Date = entry.Date.Format(time.DateOnly)
		}
		out = append(out, note)
	}
	return out
}
