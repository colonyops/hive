package review

import (
	"context"
	"fmt"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
)

// Scope is what a review view lists: the drive space whose changes refresh
// it, and the directory its documents are discovered from. Folder narrows the
// listing to one folder of the space, in the space's forward-slash paths.
// With All, the view lists every space under Dir, the drive root.
type Scope struct {
	Space  drive.Space
	Dir    string
	Folder string
	All    bool
}

// SpaceLocator finds the drive space a path on disk belongs to.
type SpaceLocator interface {
	SpaceAt(path string) (drive.Space, string, bool)
	SpaceDir(space drive.Space) (string, error)
}

// FileScope is the scope and the document of one file opened by path. A
// file inside a space lists its own folder of that space and refreshes on
// the space's changes, its path rebuilt from the space directory so the
// listing matches it. A file outside every space is listed alone.
func FileScope(locator SpaceLocator, path string, modTime time.Time) (Scope, Document) {
	alone := Document{Path: path, RelPath: filepath.Base(path), Type: TypeOther, ModTime: modTime}
	space, rel, ok := locator.SpaceAt(path)
	if !ok {
		return Scope{Dir: filepath.Dir(path)}, alone
	}
	dir, err := locator.SpaceDir(space)
	if err != nil {
		return Scope{Dir: filepath.Dir(path)}, alone
	}
	folder := pathpkg.Dir(rel)
	if folder == "." {
		folder = ""
	}
	doc := spaceDocument(space, dir, rel, false)
	doc.Type = typeFromPath(rel)
	doc.ModTime = modTime
	return Scope{Space: space, Dir: dir, Folder: folder}, doc
}

// Source is the part of the drive engine a review view lists documents from.
type Source interface {
	Spaces(ctx context.Context) ([]drive.Space, error)
	Tree(ctx context.Context, space drive.Space) (drive.Entry, error)
}

// covers reports whether a drive.changed payload for space is this scope's.
func (s Scope) covers(space string) bool {
	if s.Dir == "" {
		return false
	}
	return s.All || (s.Space.Key != "" && space == s.Space.Key)
}

// ListDocuments lists the markdown documents of the scope, sorted. Without a
// source or a directory it lists none.
func ListDocuments(ctx context.Context, source Source, scope Scope) ([]Document, error) {
	if source == nil || scope.Dir == "" {
		return nil, nil
	}
	if !scope.All {
		root, err := source.Tree(ctx, scope.Space)
		if err != nil {
			return nil, err
		}
		folder, ok := subtree(root, scope.Folder)
		if !ok {
			return nil, nil
		}
		return DocumentsFromTree(scope.Space, scope.Dir, folder, false), nil
	}

	spaces, err := source.Spaces(ctx)
	if err != nil {
		return nil, err
	}
	var docs []Document
	for _, space := range spaces {
		root, err := source.Tree(ctx, space)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", space.Key, err)
		}
		docs = append(docs, DocumentsFromTree(space, filepath.Join(scope.Dir, filepath.FromSlash(space.Key)), root, true)...)
	}
	SortDocuments(docs)
	return docs, nil
}

func subtree(entry drive.Entry, folder string) (drive.Entry, bool) {
	if entry.Path == folder {
		return entry, true
	}
	for _, child := range entry.Children {
		if child.Kind == drive.KindDir && (child.Path == folder || strings.HasPrefix(folder, child.Path+"/")) {
			return subtree(child, folder)
		}
	}
	return drive.Entry{}, false
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
	scope, source, logger := v.scope, v.source, v.logger
	return func() tea.Msg {
		docs, err := ListDocuments(context.Background(), source, scope)
		if err != nil {
			logger.Warn().Err(err).Str("dir", scope.Dir).Msg("review: documents not rediscovered")
			return DocumentChangeMsg{}
		}
		return DocumentChangeMsg{Documents: docs}
	}
}
