package review

import (
	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
)

// Scope is what a review view lists: the drive space whose changes refresh
// it, and the directory its documents are discovered from.
type Scope struct {
	Space drive.Space
	Dir   string
}

// covers reports whether a drive.changed payload for space is this scope's.
func (s Scope) covers(space string) bool {
	return s.Dir != "" && s.Space.Key != "" && space == s.Space.Key
}

// DocumentChangeMsg carries the documents discovered after the scope's space
// changed on disk, or after the scope itself changed.
type DocumentChangeMsg struct {
	Documents []Document
}

// DriveChangedMsg is one drive.changed payload from the engine bus.
type DriveChangedMsg struct {
	Payload events.DriveChangedPayload
}

// subscribeDriveChanges pushes every drive.changed payload into a channel a
// command waits on. A full channel drops the payload: each one leads to a
// whole re-discovery, so a payload still queued covers the dropped one.
func subscribeDriveChanges(logger zerolog.Logger, bus *events.EventBus) <-chan events.DriveChangedPayload {
	if bus == nil {
		return nil
	}
	ch := make(chan events.DriveChangedPayload, 16)
	bus.SubscribeDriveChanged(func(p events.DriveChangedPayload) {
		select {
		case ch <- p:
		default:
			logger.Debug().Str("space", p.Space).Msg("review: drive change dropped, channel buffer full")
		}
	})
	return ch
}

// listenForDriveChanges waits for the next payload. Nil without a bus.
func (v View) listenForDriveChanges() tea.Cmd {
	ch := v.changes
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		payload, ok := <-ch
		if !ok {
			return nil
		}
		return DriveChangedMsg{Payload: payload}
	}
}

// discover lists the scope's documents again. An empty directory lists none.
func (v View) discover() tea.Cmd {
	dir := v.scope.Dir
	logger := v.logger
	return func() tea.Msg {
		if dir == "" {
			return DocumentChangeMsg{}
		}
		docs, err := DiscoverDocuments(dir)
		if err != nil {
			logger.Warn().Err(err).Str("dir", dir).Msg("review: documents not rediscovered")
			return DocumentChangeMsg{}
		}
		return DocumentChangeMsg{Documents: docs}
	}
}
