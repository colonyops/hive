package wailsui

import (
	"context"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
)

// DriveService exposes Hive Drive to the frontend: spaces, trees, search,
// the reader, and the user's own reorganization. Every space argument is a
// space key ("owner/repo", "workspaces/<name>", "shared") and every path is
// relative to that space's root.
type DriveService struct {
	drive *app.DriveService
}

func NewDriveService(d *app.DriveService) *DriveService {
	return &DriveService{drive: d}
}

func (s *DriveService) Spaces(ctx context.Context) ([]SpaceWire, error) {
	spaces, err := s.drive.Spaces(ctx)
	if err != nil {
		return nil, err
	}
	return spaceWiresOf(spaces), nil
}

func (s *DriveService) Tree(ctx context.Context, space string) (EntryWire, error) {
	root, err := s.drive.Tree(ctx, space)
	if err != nil {
		return EntryWire{}, err
	}
	return entryWireOf(root), nil
}

// Search matches query against paths and titles in every space. limit 0 is
// the drive's default cap.
func (s *DriveService) Search(ctx context.Context, query string, limit int) ([]HitWire, error) {
	hits, err := s.drive.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return hitWiresOf(hits), nil
}

func (s *DriveService) Document(ctx context.Context, space, path string) (DocumentWire, error) {
	doc, err := s.drive.Document(ctx, space, path)
	if err != nil {
		return DocumentWire{}, err
	}
	return documentWireOf(doc), nil
}

// Markdown is a markdown document's body or a canvas rendered as markdown.
func (s *DriveService) Markdown(ctx context.Context, space, path string) (string, error) {
	return s.drive.Markdown(ctx, space, path)
}

// Export writes Markdown's answer to dest, an absolute path from the native
// save dialog.
func (s *DriveService) Export(ctx context.Context, space, path, dest string) error {
	return s.drive.Export(ctx, space, path, dest)
}

func (s *DriveService) Delete(ctx context.Context, space, path string) error {
	return s.drive.Delete(ctx, space, path)
}

func (s *DriveService) CreateFolder(ctx context.Context, space, path string) (EntryWire, error) {
	entry, err := s.drive.CreateFolder(ctx, space, path)
	if err != nil {
		return EntryWire{}, err
	}
	return entryWireOf(entry), nil
}

// Rename gives path a new name in the same folder. A document keeps its
// kind's extension: "b" renames a.md to b.md.
func (s *DriveService) Rename(ctx context.Context, space, path, name string) (EntryWire, error) {
	entry, err := s.drive.Rename(ctx, space, path, name)
	if err != nil {
		return EntryWire{}, err
	}
	return entryWireOf(entry), nil
}

// Move puts path into the folder dir under its own name; dir "" is the
// space root.
func (s *DriveService) Move(ctx context.Context, space, path, dir string) (EntryWire, error) {
	entry, err := s.drive.Move(ctx, space, path, dir)
	if err != nil {
		return EntryWire{}, err
	}
	return entryWireOf(entry), nil
}

// Locate is the absolute path on disk of path in space; path "" is the
// space's own folder.
func (s *DriveService) Locate(ctx context.Context, space, path string) (string, error) {
	return s.drive.Locate(ctx, space, path)
}

func (s *DriveService) Reveal(ctx context.Context, space, path string) error {
	return s.drive.Reveal(ctx, space, path)
}
