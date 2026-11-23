package drive

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/ops"
)

// MaxDocumentBytes caps one document put body. A document is read back whole
// into an agent's context and rendered whole in the Drive tab, so a larger one
// is a mistake on both ends.
const MaxDocumentBytes = 2 << 20

// Caller is who a document call acts for and the space its documents belong
// to.
type Caller struct {
	Space  drive.Space
	Author drive.Author
	// Session is set for a hive session caller only.
	Session *session.Session
}

// Chat is the part of a chat record a document call needs.
type Chat struct {
	ID        string
	Workspace string
}

// Chats looks up a chat by id. Chat records live in the desktop's store, so
// only the desktop provides it. A chat that does not exist is an *ops.Error of
// kind not_found.
type Chats interface {
	Chat(ctx context.Context, id string) (Chat, error)
}

// SessionCaller is a hive session's caller, filed under its repository.
func (s *Service) SessionCaller(sess session.Session) (Caller, error) {
	space, ok := s.SpaceForRemote(sess.Remote)
	if !ok {
		return Caller{}, ops.Errorf(ops.KindInvalid,
			"hive session %q has a remote with no owner and repository to file documents under", sess.Name)
	}
	return Caller{Space: space, Author: drive.Author{HiveSession: sess.ID}, Session: &sess}, nil
}

// ChatCaller is a chat's caller, filed under its workspace.
func ChatCaller(chat Chat) (Caller, error) {
	space, err := drive.WorkspaceSpace(chat.Workspace)
	if err != nil {
		return Caller{}, ops.Errorf(ops.KindInvalid, "workspace %q is not a valid workspace directory name", chat.Workspace)
	}
	return Caller{Space: space, Author: drive.Author{Session: chat.ID}}, nil
}

// ListDocuments is the entry at path in the caller's space: a folder with one
// level of children, or its whole subtree when tree is set. "" is the space
// root, and a document path answers that document alone.
func (s *Service) ListDocuments(ctx context.Context, c Caller, path string, tree bool) (drive.Entry, error) {
	path = strings.Trim(path, "/")
	if path != "" {
		if err := drive.ValidatePath(path); err != nil {
			return drive.Entry{}, documentError(err, c.Space, path)
		}
	}
	entry, err := s.Entry(ctx, c.Space, path)
	if errors.Is(err, drive.ErrNotFound) {
		return drive.Entry{}, ops.Errorf(ops.KindNotFound, "nothing at %q in this space; an empty path lists the space root", path)
	}
	if err != nil {
		return drive.Entry{}, documentError(err, c.Space, path)
	}
	if !tree {
		for i := range entry.Children {
			entry.Children[i].Children = nil
		}
	}
	return entry, nil
}

// GetDocument is one markdown, html, or plain UTF-8 file and its bytes, as
// stored. Hive's own state files answer not_found, as they are absent from
// every listing.
func (s *Service) GetDocument(ctx context.Context, c Caller, path string) (drive.Entry, []byte, error) {
	if err := drive.ValidatePath(path); err != nil {
		return drive.Entry{}, nil, documentError(err, c.Space, path)
	}
	if drive.IsHiveState(path) {
		return drive.Entry{}, nil, documentNotFound(c.Space, path)
	}
	isDir, err := s.isDir(c.Space, path)
	if err != nil {
		return drive.Entry{}, nil, err
	}
	if isDir {
		return drive.Entry{}, nil, ops.Errorf(ops.KindInvalid, "%q is a folder; list it instead", path)
	}
	entry, body, err := s.Read(ctx, c.Space, path)
	if err != nil {
		return drive.Entry{}, nil, documentError(err, c.Space, path)
	}
	if !utf8.Valid(body) {
		return drive.Entry{}, nil, ops.Errorf(ops.KindInvalid, "%q is not a text file", path)
	}
	return entry, body, nil
}

// PutDocument replaces a markdown or html document with body, creating it and
// the folders above it as needed. Every surface refuses a body by these
// checks, so the desktop and the CLI give the same message.
func (s *Service) PutDocument(ctx context.Context, c Caller, path string, body []byte) (drive.Entry, error) {
	if err := drive.ValidatePath(path); err != nil {
		return drive.Entry{}, documentError(err, c.Space, path)
	}
	kind := drive.KindOf(path)
	if kind != drive.KindMarkdown && kind != drive.KindHtml {
		return drive.Entry{}, ops.Errorf(ops.KindInvalid, "%q is neither a markdown (.md) nor an html (.html) document", path)
	}
	if len(body) > MaxDocumentBytes {
		return drive.Entry{}, ops.Errorf(ops.KindInvalid, "document is too large (%d bytes max)", MaxDocumentBytes)
	}
	if !utf8.Valid(body) {
		return drive.Entry{}, ops.Errorf(ops.KindInvalid, "document body is not valid UTF-8")
	}
	if kind == drive.KindMarkdown {
		if err := checkFrontmatter(body); err != nil {
			return drive.Entry{}, err
		}
	}
	entry, err := s.Write(ctx, c.Space, path, c.Author, body)
	if err != nil {
		return drive.Entry{}, documentError(err, c.Space, path)
	}
	return entry, nil
}

// checkFrontmatter refuses front matter that does not parse, and front matter
// that closes past what a scan reads, which would leave the document silently
// untitled.
func checkFrontmatter(body []byte) error {
	_, present, err := drive.ParseMarkdownFrontmatter(body)
	if err != nil {
		return ops.Errorf(ops.KindInvalid, "the document's front matter does not parse: %v", err)
	}
	if !present || len(body) <= frontmatterHeadBytes {
		return nil
	}
	if _, _, err := drive.ParseMarkdownFrontmatter(body[:frontmatterHeadBytes]); err != nil {
		return ops.Errorf(ops.KindInvalid, "front matter must close within the first %d bytes of the document", frontmatterHeadBytes)
	}
	return nil
}

// MoveDocument renames a document or a folder inside the caller's space. The
// drive refuses a move that changes a document's kind.
func (s *Service) MoveDocument(ctx context.Context, c Caller, from, to string) (drive.Entry, error) {
	for _, p := range []string{from, to} {
		if err := drive.ValidatePath(p); err != nil {
			return drive.Entry{}, documentError(err, c.Space, p)
		}
	}
	entry, err := s.Move(ctx, c.Space, from, to, c.Author)
	if err != nil {
		return drive.Entry{}, moveError(err, c.Space, from, to)
	}
	return entry, nil
}

// DeleteDocument removes one document or an empty folder.
func (s *Service) DeleteDocument(ctx context.Context, c Caller, path string) error {
	if err := drive.ValidatePath(path); err != nil {
		return documentError(err, c.Space, path)
	}
	if err := s.Delete(ctx, c.Space, path, c.Author); err != nil {
		return documentError(err, c.Space, path)
	}
	return nil
}

// isDir reads the disk rather than the snapshot, which can trail a change
// made outside hive by the watcher's debounce.
func (s *Service) isDir(space drive.Space, path string) (bool, error) {
	full, err := s.Locate(space, path)
	if err != nil {
		return false, documentError(err, space, path)
	}
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return false, documentNotFound(space, path)
	}
	if err != nil {
		return false, ops.Errorf(ops.KindInternal, "reading %q in %s: %v", path, space.Key, err)
	}
	return info.IsDir(), nil
}

func documentNotFound(space drive.Space, path string) error {
	return ops.Errorf(ops.KindNotFound, "no %q in %s", path, space.Key)
}

// moveError keeps the drive's own reason for refusing a move as the message:
// it names the rule the new name broke, such as keeping the extension. A
// conflict names the target that already exists; anything else names the
// source.
func moveError(err error, space drive.Space, from, to string) error {
	switch {
	case errors.Is(err, drive.ErrInvalidPath):
		return ops.Errorf(ops.KindInvalid, "%s", strings.TrimPrefix(err.Error(), drive.ErrInvalidPath.Error()+": "))
	case errors.Is(err, drive.ErrExists):
		return documentError(err, space, to)
	default:
		return documentError(err, space, from)
	}
}

func documentError(err error, space drive.Space, path string) error {
	switch {
	case errors.Is(err, drive.ErrNotFound):
		return documentNotFound(space, path)
	case errors.Is(err, drive.ErrExists):
		return ops.Errorf(ops.KindConflict, "%q already exists in %s", path, space.Key)
	case errors.Is(err, drive.ErrNotEmpty):
		return ops.Errorf(ops.KindInvalid, "folder %q is not empty; move or delete what is in it first", path)
	case errors.Is(err, drive.ErrInvalidPath), errors.Is(err, drive.ErrInvalidSpace), errors.Is(err, drive.ErrNotWritable):
		return ops.Errorf(ops.KindInvalid, "%q in %s: %v", path, space.Key, err)
	default:
		return ops.Errorf(ops.KindInternal, "%q in %s: %v", path, space.Key, err)
	}
}
