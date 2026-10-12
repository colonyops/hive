package app

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/drive"
)

// maxDocumentBytes caps one write_document body; the tool description states it. A document is read back
// whole into an agent's context and rendered whole in the pane, so a larger
// one is a mistake on both ends.
const maxDocumentBytes = 2 << 20

// frontmatterIndexBytes is how much of a markdown file the engine reads to
// index its front matter. Front matter that closes past it is never indexed,
// so a write that would be silently untitled is refused instead.
const frontmatterIndexBytes = 8 << 10

// ListDocuments is the calling session's space and the entry at path in it: a
// folder with one level of children, or its whole subtree when tree is set.
// "" is the space root. A document path answers that document alone.
func (s *DriveService) ListDocuments(ctx context.Context, session, path string, tree bool) (drive.Space, drive.Entry, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Space{}, drive.Entry{}, err
	}
	path = strings.Trim(path, "/")
	if path != "" {
		if err := drive.ValidatePath(path); err != nil {
			return drive.Space{}, drive.Entry{}, documentError(err, caller.space, path)
		}
	}
	root, err := d.Tree(ctx, caller.space)
	if err != nil {
		return drive.Space{}, drive.Entry{}, documentError(err, caller.space, path)
	}
	entry, ok := drive.Find(root, path)
	if !ok {
		return drive.Space{}, drive.Entry{}, Errorf(KindNotFound, "nothing at %q in this space; list_documents with an empty path shows the root", path)
	}
	if !tree {
		for i := range entry.Children {
			entry.Children[i].Children = nil
		}
	}
	return caller.space, entry, nil
}

// ReadDocument is one document and its bytes, as stored. A canvas is a
// structure of blocks rather than a body, so it is read with read_canvas.
func (s *DriveService) ReadDocument(ctx context.Context, session, path string) (drive.Entry, []byte, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Entry{}, nil, err
	}
	if err := drive.ValidatePath(path); err != nil {
		return drive.Entry{}, nil, documentError(err, caller.space, path)
	}
	if drive.KindOf(path) == drive.KindCanvas {
		return drive.Entry{}, nil, Errorf(KindInvalid, "%q is a canvas; read it with read_canvas and the name %q", path, drive.CanvasName(path))
	}
	entry, body, err := d.Read(ctx, caller.space, path)
	if err != nil {
		if errors.Is(err, drive.ErrInvalidPath) {
			return drive.Entry{}, nil, Errorf(KindInvalid, "%q is a folder; list it with list_documents", path)
		}
		return drive.Entry{}, nil, documentError(err, caller.space, path)
	}
	if !utf8.Valid(body) {
		return drive.Entry{}, nil, Errorf(KindInvalid, "%q is not a text file", path)
	}
	return entry, body, nil
}

// WriteDocument replaces a markdown or html document with body, creating it
// and the folders above it as needed.
func (s *DriveService) WriteDocument(ctx context.Context, session, path string, body []byte) (drive.Entry, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Entry{}, err
	}
	if err := drive.ValidatePath(path); err != nil {
		return drive.Entry{}, documentError(err, caller.space, path)
	}
	switch drive.KindOf(path) {
	case drive.KindMarkdown, drive.KindHtml:
	case drive.KindCanvas:
		return drive.Entry{}, Errorf(KindInvalid, "%q is a canvas; write its blocks with put_blocks and the name %q", path, drive.CanvasName(path))
	default:
		return drive.Entry{}, Errorf(KindInvalid, "%q is neither a markdown (.md) nor an html (.html) document", path)
	}
	if len(body) > maxDocumentBytes {
		return drive.Entry{}, Errorf(KindInvalid, "document is too large (%d bytes max)", maxDocumentBytes)
	}
	if !utf8.Valid(body) {
		return drive.Entry{}, Errorf(KindInvalid, "document body is not valid UTF-8")
	}
	if drive.KindOf(path) == drive.KindMarkdown {
		if err := checkFrontmatter(body); err != nil {
			return drive.Entry{}, err
		}
	}
	entry, err := d.Write(ctx, caller.space, path, caller.author, body)
	if err != nil {
		return drive.Entry{}, documentError(err, caller.space, path)
	}
	return entry, nil
}

func checkFrontmatter(body []byte) error {
	_, present, err := drive.ParseMarkdownFrontmatter(body)
	if err != nil {
		return Wrap(err, KindInvalid, "the document's front matter does not parse")
	}
	if !present || len(body) <= frontmatterIndexBytes {
		return nil
	}
	if _, _, err := drive.ParseMarkdownFrontmatter(body[:frontmatterIndexBytes]); err != nil {
		return Errorf(KindInvalid, "front matter must close within the first %d bytes of the document", frontmatterIndexBytes)
	}
	return nil
}

// MoveDocument renames a document or a folder inside the calling session's
// space. A document keeps its kind: renaming plan.md to plan.html would hand
// one renderer the other's source.
func (s *DriveService) MoveDocument(ctx context.Context, session, from, to string) (drive.Entry, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Entry{}, err
	}
	for _, p := range []string{from, to} {
		if err := drive.ValidatePath(p); err != nil {
			return drive.Entry{}, documentError(err, caller.space, p)
		}
	}
	if drive.KindOf(from) != drive.KindOf(to) {
		return drive.Entry{}, Errorf(KindInvalid, "moving %q to %q would change its kind; keep the extension", from, to)
	}
	entry, err := d.Move(ctx, caller.space, from, to, caller.author)
	if err != nil {
		return drive.Entry{}, documentError(err, caller.space, from)
	}
	return entry, nil
}

// DeleteDocument removes one document, a canvas included, or an empty
// folder.
func (s *DriveService) DeleteDocument(ctx context.Context, session, path string) error {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return err
	}
	if err := drive.ValidatePath(path); err != nil {
		return documentError(err, caller.space, path)
	}
	if err := d.Delete(ctx, caller.space, path, caller.author); err != nil {
		return documentError(err, caller.space, path)
	}
	return nil
}

// OpenDocument asks the UI to open the drive pane beside the calling chat or
// Code session, pinned to path when one is given. Like SetPaneOpen it is UI
// intent, applied only while the user is viewing that chat or session.
func (s *DriveService) OpenDocument(ctx context.Context, session, path string) error {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return err
	}
	if caller.space.Kind == drive.SpaceKindShared {
		return Errorf(KindInvalid, "a global canvas has no pane to open or close; the user reads it in the full-page canvas view")
	}
	if path != "" {
		if err := drive.ValidatePath(path); err != nil {
			return documentError(err, caller.space, path)
		}
		if _, _, err := d.Read(ctx, caller.space, path); err != nil {
			if errors.Is(err, drive.ErrInvalidPath) {
				return Errorf(KindInvalid, "%q is a folder; open a document inside it", path)
			}
			return documentError(err, caller.space, path)
		}
	}
	s.events.Publish(ctx, events.DriveToggleRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession,
		Space: caller.space.Key, Path: path, Open: true,
	})
	return nil
}
