package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	"github.com/colonyops/hive/pkg/osopen"
)

// maxCanvasBodyBytes caps one markdown or html block's body so a single tool
// call cannot make the pane unrenderable.
const maxCanvasBodyBytes = 256 * 1024

const (
	maxCanvasBlockIDLength = 200
	maxCanvasTitleLength   = 200
	// maxCanvasBatchBlocks caps one put_blocks call; a larger layout is
	// written in slices.
	maxCanvasBatchBlocks = 50
)

// canvasSessionResolver is the one store read a chat's canvas mutation starts
// with: the session record is the authority on which workspace a canvas
// belongs to, so an agent never names the workspace itself.
type canvasSessionResolver interface {
	Get(ctx context.Context, id string) (stores.AgentSession, error)
}

// DriveService is the desktop's door onto Hive Drive. Agents read and write
// documents and canvas blocks through the hive-drive MCP tools; the UI reads
// and reorganizes a space by its key.
type DriveService struct {
	// engine is read per call: it starts after the services are built, and
	// every method answers KindUnavailable until it has.
	engine   func() *hive.Engine
	sessions canvasSessionResolver
	events   *events.Bus
	reveal   func(string) error
	logger   zerolog.Logger
}

type DriveDeps struct {
	Engine   func() *hive.Engine
	Sessions canvasSessionResolver
	Events   *events.Bus
	Logger   zerolog.Logger
	// Reveal shows an absolute path in the file manager; nil is osopen.Reveal.
	Reveal func(string) error
}

func newDriveService(d DriveDeps) *DriveService {
	if d.Reveal == nil {
		d.Reveal = osopen.Reveal
	}
	return &DriveService{engine: d.Engine, sessions: d.Sessions, events: d.Events, reveal: d.Reveal, logger: d.Logger}
}

// driveCaller is who a tool call comes from and the space its canvases
// belong to.
type driveCaller struct {
	space  drive.Space
	author drive.Author
}

// GetCanvas is one canvas in the calling session's space. A name nothing
// was ever written under is not_found: unlike the pane, an agent asking by
// name should learn the name is wrong, not see a blank surface.
func (s *DriveService) GetCanvas(ctx context.Context, session, name string) (drive.Canvas, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Canvas{}, err
	}
	path, err := canvasPath(name)
	if err != nil {
		return drive.Canvas{}, err
	}
	c, ok, err := d.LoadCanvas(ctx, caller.space, path)
	if err != nil {
		return drive.Canvas{}, driveError(err, name)
	}
	if !ok {
		return drive.Canvas{}, canvasNotFound(name)
	}
	return s.withCanonicalAuthor(ctx, c)
}

// PutBlock creates or replaces one block, creating the canvas on its first
// write. A non-empty title renames the canvas; empty leaves the stored one.
// A non-empty before places the block ahead of that existing block id
// instead of appending.
func (s *DriveService) PutBlock(ctx context.Context, session, name, title, before string, b drive.Block) (drive.Canvas, error) {
	return s.putBlocks(ctx, session, name, title, before, []drive.Block{b})
}

// PutBlocks writes a batch of blocks as one canvas write: one file write,
// one pane render. Every block is validated before any is written, so a
// rejected batch leaves the canvas untouched.
func (s *DriveService) PutBlocks(ctx context.Context, session, name, title string, blocks []drive.Block) (drive.Canvas, error) {
	if len(blocks) == 0 {
		return drive.Canvas{}, Errorf(KindInvalid, "a batch needs at least one block")
	}
	if len(blocks) > maxCanvasBatchBlocks {
		return drive.Canvas{}, Errorf(KindInvalid, "too many blocks in one batch (%d max)", maxCanvasBatchBlocks)
	}
	return s.putBlocks(ctx, session, name, title, "", blocks)
}

func (s *DriveService) putBlocks(ctx context.Context, session, name, title, before string, blocks []drive.Block) (drive.Canvas, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Canvas{}, err
	}
	path, err := canvasPath(name)
	if err != nil {
		return drive.Canvas{}, err
	}
	title = plainTitle(title)
	if len(title) > maxCanvasTitleLength {
		return drive.Canvas{}, Errorf(KindInvalid, "canvas title is too long (%d chars max)", maxCanvasTitleLength)
	}
	seen := make(map[string]bool, len(blocks))
	for i := range blocks {
		if err := validateBlock(&blocks[i]); err != nil {
			if len(blocks) > 1 {
				return drive.Canvas{}, Wrap(err, KindInvalid, "blocks[%d]", i)
			}
			return drive.Canvas{}, err
		}
		if seen[blocks[i].ID] {
			return drive.Canvas{}, Errorf(KindInvalid, "block id %q appears twice in one batch", blocks[i].ID)
		}
		seen[blocks[i].ID] = true
	}
	c, err := d.UpsertBlocks(ctx, caller.space, path, caller.author, title, before, blocks...)
	if err != nil {
		if errors.Is(err, drive.ErrAnchorNotFound) {
			return drive.Canvas{}, Errorf(KindNotFound, "no block %q on canvas %q to place before", before, name)
		}
		return drive.Canvas{}, driveError(err, name)
	}
	return s.withCanonicalAuthor(ctx, c)
}

// RemoveBlock deletes one block by id and answers the canvas that remains.
func (s *DriveService) RemoveBlock(ctx context.Context, session, name, blockID string) (drive.Canvas, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Canvas{}, err
	}
	path, err := canvasPath(name)
	if err != nil {
		return drive.Canvas{}, err
	}
	c, removed, err := d.RemoveBlock(ctx, caller.space, path, blockID, caller.author)
	if err != nil {
		return drive.Canvas{}, driveError(err, name)
	}
	if !removed {
		return drive.Canvas{}, Errorf(KindNotFound, "no block %q on canvas %q", blockID, name)
	}
	return s.withCanonicalAuthor(ctx, c)
}

// SetFrontmatter replaces the editable front matter on an existing canvas.
// created_at and updated_at stay derived from the canvas timestamps.
func (s *DriveService) SetFrontmatter(ctx context.Context, session, name string, fm drive.Frontmatter) (drive.Canvas, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Canvas{}, err
	}
	path, err := canvasPath(name)
	if err != nil {
		return drive.Canvas{}, err
	}
	if err := drive.ValidateFrontmatter(fm); err != nil {
		return drive.Canvas{}, Wrap(err, KindInvalid, "front matter")
	}
	c, err := d.SetCanvasFrontmatter(ctx, caller.space, path, caller.author, fm)
	if err != nil {
		return drive.Canvas{}, driveError(err, name)
	}
	return s.withCanonicalAuthor(ctx, c)
}

// ClearCanvas removes every block at once; the canvas, its title and its
// file survive.
func (s *DriveService) ClearCanvas(ctx context.Context, session, name string) (drive.Canvas, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return drive.Canvas{}, err
	}
	path, err := canvasPath(name)
	if err != nil {
		return drive.Canvas{}, err
	}
	c, err := d.ClearCanvas(ctx, caller.space, path, caller.author)
	if err != nil {
		return drive.Canvas{}, driveError(err, name)
	}
	return s.withCanonicalAuthor(ctx, c)
}

func (s *DriveService) DeleteCanvas(ctx context.Context, session, name string) error {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return err
	}
	path, err := canvasPath(name)
	if err != nil {
		return err
	}
	if err := d.Delete(ctx, caller.space, path, caller.author); err != nil {
		return driveError(err, name)
	}
	return nil
}

// ListCanvases is the calling session's space, most recently updated first.
func (s *DriveService) ListCanvases(ctx context.Context, session string) ([]drive.CanvasMeta, error) {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return nil, err
	}
	return s.listCanvases(ctx, d, caller.space)
}

// SetPaneOpen asks the UI to open or close the canvas pane beside the
// calling chat or Code session. It is UI intent, applied only while the user
// is viewing it, so an agent can surface what it made without ever dragging
// the user away from something else. Opening with a name requires that
// canvas to exist; empty leaves the pane's own pick.
func (s *DriveService) SetPaneOpen(ctx context.Context, session, name string, open bool) error {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return err
	}
	if caller.space.Kind == drive.SpaceKindShared {
		return Errorf(KindInvalid, "the shared space has no pane to open or close; the user reads it in the Drive page")
	}
	pinned := ""
	if open && name != "" {
		path, err := canvasPath(name)
		if err != nil {
			return err
		}
		c, ok, err := d.LoadCanvas(ctx, caller.space, path)
		if err != nil {
			return driveError(err, name)
		}
		if !ok {
			return canvasNotFound(name)
		}
		pinned = c.Path
	}
	s.events.Publish(ctx, events.DriveToggleRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession,
		Space: caller.space.Key, Path: pinned, Open: open,
	})
	return nil
}

// GetCanvasAt is the pane's read: by space key and canvas name or path. A
// canvas nothing was written under answers empty rather than an error, so
// the pane never flashes a failure for one deleted while it was open. html
// blocks leave here sanitized: this is the one seam between stored agent
// source and the app's own webview, so the frontend holds no policy of its
// own (ADR canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary).
func (s *DriveService) GetCanvasAt(ctx context.Context, space, path string) (drive.Canvas, error) {
	t, err := s.target(space, path)
	if err != nil {
		return drive.Canvas{}, err
	}
	c, ok, err := t.svc.LoadCanvas(ctx, t.space, t.path)
	if err != nil {
		return drive.Canvas{}, driveError(err, path)
	}
	if !ok {
		return drive.Canvas{Space: t.space.Key, Path: t.path, Frontmatter: drive.Frontmatter{}, Blocks: []drive.Block{}}, nil
	}
	for i, b := range c.Blocks {
		if b.Kind == drive.BlockKindHTML {
			c.Blocks[i].Body = canvas.SanitizeHTML(b.Body)
		}
	}
	return s.withCanonicalAuthor(ctx, c)
}

// Markdown is one document as standalone markdown, the reader's copy action:
// a markdown document's own body, or a canvas rendered. A canvas nothing was
// written under is not_found: exporting nothing is a mistake worth surfacing,
// unlike showing it. An html document or a plain file has no markdown form
// and is refused here, so the policy does not live only in the frontend.
func (s *DriveService) Markdown(ctx context.Context, space, path string) (string, error) {
	switch drive.KindOf(path) {
	case drive.KindMarkdown:
		d, parsed, err := s.space(space)
		if err != nil {
			return "", err
		}
		_, body, err := d.Read(ctx, parsed, path)
		if err != nil {
			return "", documentError(err, parsed, path)
		}
		return string(body), nil
	case drive.KindHtml:
		return "", Errorf(KindInvalid, "%q is an html document; only markdown documents and canvases export as markdown", path)
	case drive.KindFile:
		// A bare canvas name such as "plan" has a plain file's kind by its
		// extension, so only a plain file that exists at that exact path is
		// refused.
		d, parsed, err := s.space(space)
		if err != nil {
			return "", err
		}
		if path != "" && drive.ValidatePath(path) == nil {
			entry, err := d.Entry(ctx, parsed, path)
			switch {
			case err == nil && entry.Kind == drive.KindFile:
				return "", Errorf(KindInvalid, "%q is a plain file; only markdown documents and canvases export as markdown", path)
			case err != nil && !errors.Is(err, drive.ErrNotFound):
				return "", documentError(err, parsed, path)
			}
		}
	case drive.KindCanvas, drive.KindDir:
	}
	t, err := s.target(space, path)
	if err != nil {
		return "", err
	}
	c, ok, err := t.svc.LoadCanvas(ctx, t.space, t.path)
	if err != nil {
		return "", driveError(err, path)
	}
	if !ok {
		return "", canvasNotFound(path)
	}
	markdown, err := canvas.Markdown(c)
	if err != nil {
		return "", Wrap(err, KindInternal, "rendering canvas %q as markdown", path)
	}
	return markdown, nil
}

// Export writes one document's Markdown to dest, the pane's save
// action. dest comes from the native save dialog, which is why it must
// already be absolute.
func (s *DriveService) Export(ctx context.Context, space, path, dest string) error {
	markdown, err := s.Markdown(ctx, space, path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(dest) {
		return Errorf(KindInvalid, "export path must be absolute")
	}
	if err := os.WriteFile(dest, []byte(markdown), 0o644); err != nil {
		return Wrap(err, KindInternal, "writing export for %q", path)
	}
	return nil
}

// DocumentView is one document as the reader shows it. Exactly one of
// Markdown, HTML, and Canvas is set for those kinds; a plain file carries
// only its entry.
type DocumentView struct {
	Space    drive.Space   `json:"space"`
	Entry    drive.Entry   `json:"entry"`
	Markdown string        `json:"markdown,omitempty"` // raw: the frontend's sanitizing renderer handles it
	HTML     string        `json:"html,omitempty"`     // CSP meta injected first
	Canvas   *drive.Canvas `json:"canvas,omitempty"`   // html blocks sanitized
}

// Spaces is every space in the drive: repositories, then workspaces, then
// shared.
func (s *DriveService) Spaces(ctx context.Context) ([]drive.Space, error) {
	d, err := s.engineDrive()
	if err != nil {
		return nil, err
	}
	spaces, err := d.Spaces(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing drive spaces")
	}
	return spaces, nil
}

// Tree is one space's whole folder tree. A space with no directory yet is an
// empty root.
func (s *DriveService) Tree(ctx context.Context, space string) (drive.Entry, error) {
	d, parsed, err := s.space(space)
	if err != nil {
		return drive.Entry{}, err
	}
	root, err := d.Tree(ctx, parsed)
	if err != nil {
		return drive.Entry{}, documentError(err, parsed, "")
	}
	return root, nil
}

// Search matches query against every space's paths and titles; limit 0 is
// the drive's default cap.
func (s *DriveService) Search(ctx context.Context, query string, limit int) ([]drivesvc.Hit, error) {
	d, err := s.engineDrive()
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, Errorf(KindInvalid, "search limit must not be negative")
	}
	hits, err := d.Search(ctx, query, limit)
	if err != nil {
		return nil, Wrap(err, KindInternal, "searching the drive")
	}
	return hits, nil
}

// Document is one document read for the reader. The kind comes from the
// name, so a plain file is answered from the snapshot without reading what
// may be a large binary, and a canvas is read once, by the canvas load that
// parses it.
func (s *DriveService) Document(ctx context.Context, space, path string) (DocumentView, error) {
	d, parsed, err := s.space(space)
	if err != nil {
		return DocumentView{}, err
	}
	if path == "" {
		return DocumentView{}, Errorf(KindInvalid, "the space root is a folder, not a document")
	}
	if err := drive.ValidatePath(path); err != nil {
		return DocumentView{}, Errorf(KindInvalid, "%q is not a path inside the space", path)
	}
	view := DocumentView{Space: parsed}
	switch drive.KindOf(path) {
	case drive.KindFile, drive.KindCanvas:
		entry, err := d.Entry(ctx, parsed, path)
		if err != nil {
			return DocumentView{}, documentError(err, parsed, path)
		}
		if entry.Kind == drive.KindDir {
			return DocumentView{}, Errorf(KindInvalid, "%q is a folder, not a document", path)
		}
		view.Entry = entry
		if entry.Kind == drive.KindCanvas {
			c, err := s.GetCanvasAt(ctx, space, path)
			if err != nil {
				return DocumentView{}, err
			}
			view.Canvas = &c
		}
		return view, nil
	case drive.KindMarkdown, drive.KindHtml, drive.KindDir:
	}
	entry, body, err := d.Read(ctx, parsed, path)
	if err != nil {
		return DocumentView{}, documentError(err, parsed, path)
	}
	view.Entry = entry
	if entry.Kind == drive.KindHtml {
		view.HTML = canvas.WrapDocument(body)
	} else {
		view.Markdown = string(body)
	}
	return view, nil
}

// CreateFolder makes one folder, and any folders above it, in a space.
func (s *DriveService) CreateFolder(ctx context.Context, space, path string) (drive.Entry, error) {
	d, parsed, err := s.space(space)
	if err != nil {
		return drive.Entry{}, err
	}
	if err := documentPath(path); err != nil {
		return drive.Entry{}, err
	}
	entry, err := d.CreateFolder(ctx, parsed, path, drive.Author{})
	if err != nil {
		return drive.Entry{}, documentError(err, parsed, path)
	}
	return entry, nil
}

// Rename gives a document or folder a new name in the same folder. name is a
// name, not a path. A name with no extension takes the document's: "b"
// renames a.md to b.md. The drive refuses "b.html", which would turn a
// markdown document into an html one.
func (s *DriveService) Rename(ctx context.Context, space, path, name string) (drive.Entry, error) {
	d, parsed, err := s.space(space)
	if err != nil {
		return drive.Entry{}, err
	}
	if err := documentPath(path); err != nil {
		return drive.Entry{}, err
	}
	if strings.Contains(name, "/") || drive.ValidatePath(name) != nil {
		return drive.Entry{}, Errorf(KindInvalid, "%q is not a valid name: use one name with no slash that does not start with a dot", name)
	}
	isDir, err := s.isDir(d, parsed, path)
	if err != nil {
		return drive.Entry{}, err
	}
	if !isDir {
		name = withDocumentExt(path, name)
	}
	return s.move(ctx, d, parsed, path, joinDrivePath(parentDrivePath(path), name))
}

// Move puts a document or folder into dir, a folder in the same space, under
// its own name. dir "" is the space root.
func (s *DriveService) Move(ctx context.Context, space, path, dir string) (drive.Entry, error) {
	d, parsed, err := s.space(space)
	if err != nil {
		return drive.Entry{}, err
	}
	if err := documentPath(path); err != nil {
		return drive.Entry{}, err
	}
	if dir != "" {
		if err := documentPath(dir); err != nil {
			return drive.Entry{}, err
		}
		isDir, err := s.isDir(d, parsed, dir)
		if err != nil {
			return drive.Entry{}, err
		}
		if !isDir {
			return drive.Entry{}, Errorf(KindInvalid, "%q is a document, not a folder to move into", dir)
		}
	}
	return s.move(ctx, d, parsed, path, joinDrivePath(dir, baseDrivePath(path)))
}

// move is a rename that is a no-op when nothing changes, so a rename
// committed without an edit, or a move into the current folder, is not an
// error.
func (s *DriveService) move(ctx context.Context, d *drivesvc.Service, space drive.Space, from, to string) (drive.Entry, error) {
	if from == to {
		entry, err := d.Entry(ctx, space, from)
		if err != nil {
			return drive.Entry{}, documentError(err, space, from)
		}
		return entry, nil
	}
	entry, err := d.Move(ctx, space, from, to, drive.Author{})
	if err != nil {
		return drive.Entry{}, moveError(err, space, from, to)
	}
	return entry, nil
}

// Delete removes a document or an empty folder. A canvas publishes with the
// author that wrote it, so the pane beside that chat or session wakes.
func (s *DriveService) Delete(ctx context.Context, space, path string) error {
	d, parsed, err := s.space(space)
	if err != nil {
		return err
	}
	if err := documentPath(path); err != nil {
		return err
	}
	var author drive.Author
	if drive.KindOf(path) == drive.KindCanvas {
		// A canvas that no longer parses is still deleted, just without an
		// author to wake.
		c, ok, err := d.LoadCanvas(ctx, parsed, path)
		switch {
		case err != nil:
			s.logger.Debug().Err(err).Str("space", parsed.Key).Str("path", path).Msg("deleting a canvas that does not load; no author to wake")
		case ok:
			author = drive.Author{Session: c.Session, HiveSession: c.HiveSession}
		}
	}
	if err := d.Delete(ctx, parsed, path, author); err != nil {
		return documentError(err, parsed, path)
	}
	return nil
}

// Locate is the absolute path on disk of a document or folder in a space;
// path "" is the space's own folder.
func (s *DriveService) Locate(_ context.Context, space, path string) (string, error) {
	d, parsed, err := s.space(space)
	if err != nil {
		return "", err
	}
	if path != "" && drive.ValidatePath(path) != nil {
		return "", Errorf(KindInvalid, "%q is not a path inside the space", path)
	}
	full, err := d.Locate(parsed, path)
	if err != nil {
		return "", documentError(err, parsed, path)
	}
	if _, err := os.Lstat(full); errors.Is(err, fs.ErrNotExist) {
		return "", documentNotFound(parsed, path)
	} else if err != nil {
		return "", Wrap(err, KindInternal, "reading %q in %s", path, parsed.Key)
	}
	return full, nil
}

// Reveal shows a document or folder in the OS file manager.
func (s *DriveService) Reveal(ctx context.Context, space, path string) error {
	full, err := s.Locate(ctx, space, path)
	if err != nil {
		return err
	}
	return Wrap(s.reveal(full), KindInternal, "revealing %s", full)
}

func (s *DriveService) space(key string) (*drivesvc.Service, drive.Space, error) {
	d, err := s.engineDrive()
	if err != nil {
		return nil, drive.Space{}, err
	}
	parsed, err := parseSpace(key)
	if err != nil {
		return nil, drive.Space{}, err
	}
	return d, parsed, nil
}

// isDir reads the disk rather than the snapshot: a rename or move decides the
// target's name from it, and the snapshot can trail a change made outside
// the app by the watcher's debounce.
func (s *DriveService) isDir(d *drivesvc.Service, space drive.Space, path string) (bool, error) {
	full, err := d.Locate(space, path)
	if err != nil {
		return false, documentError(err, space, path)
	}
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return false, documentNotFound(space, path)
	}
	if err != nil {
		return false, Wrap(err, KindInternal, "reading %q in %s", path, space.Key)
	}
	return info.IsDir(), nil
}

// withDocumentExt is the file name a document renamed to name gets: name with
// the document's extension appended when name has none. Any other name is
// left for the drive's Move to accept or refuse.
func withDocumentExt(path, name string) string {
	kind := drive.KindOf(path)
	switch got := drive.KindOf(name); {
	case got == drive.KindFile && kind != drive.KindFile:
		return name + documentExt(baseDrivePath(path), kind)
	case got == drive.KindFile && pathpkg.Ext(name) == "":
		return name + pathpkg.Ext(path)
	}
	return name
}

// documentExt is the extension that makes a name the given kind, read from
// the document's own name so its case is kept.
func documentExt(name string, kind drive.Kind) string {
	if kind == drive.KindCanvas {
		return name[len(name)-len(drive.CanvasExt):]
	}
	return pathpkg.Ext(name)
}

// documentPath refuses the space root and anything outside the space before
// the drive is asked, so the message names the mistake rather than a file.
func documentPath(path string) error {
	if path == "" {
		return Errorf(KindInvalid, "the space root cannot be changed")
	}
	if err := drive.ValidatePath(path); err != nil {
		return Errorf(KindInvalid, "%q is not a path inside the space", path)
	}
	return nil
}

func joinDrivePath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func parentDrivePath(path string) string {
	dir := pathpkg.Dir(path)
	if dir == "." {
		return ""
	}
	return dir
}

func baseDrivePath(path string) string {
	return pathpkg.Base(path)
}

func documentNotFound(space drive.Space, path string) error {
	return Errorf(KindNotFound, "no %q in %s", path, space.Key)
}

// moveError keeps the drive's own reason for refusing a move as the message,
// since the UI shows only the message: it names the rule the new name broke,
// such as keeping the extension or a canvas name the tools can address. A
// conflict names the target that already exists; anything else names the
// source.
func moveError(err error, space drive.Space, from, to string) error {
	switch {
	case errors.Is(err, drive.ErrInvalidPath):
		return Errorf(KindInvalid, "%s", strings.TrimPrefix(err.Error(), drive.ErrInvalidPath.Error()+": "))
	case errors.Is(err, drive.ErrExists):
		return documentError(err, space, to)
	default:
		return documentError(err, space, from)
	}
}

// documentError maps the drive's sentinel errors for every caller that
// addresses a document by space and path rather than a canvas by name.
func documentError(err error, space drive.Space, path string) error {
	switch {
	case errors.Is(err, drive.ErrNotFound):
		return documentNotFound(space, path)
	case errors.Is(err, drive.ErrExists):
		return Errorf(KindConflict, "%q already exists in %s", path, space.Key)
	case errors.Is(err, drive.ErrNotEmpty):
		return Errorf(KindInvalid, "folder %q is not empty; move or delete what is in it first", path)
	case errors.Is(err, drive.ErrInvalidPath), errors.Is(err, drive.ErrInvalidSpace), errors.Is(err, drive.ErrNotWritable):
		return Wrap(err, KindInvalid, "%q in %s", path, space.Key)
	default:
		return Wrap(err, KindInternal, "%q in %s", path, space.Key)
	}
}

func (s *DriveService) listCanvases(ctx context.Context, d *drivesvc.Service, space drive.Space) ([]drive.CanvasMeta, error) {
	metas, err := d.ListCanvases(ctx, space)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing canvases in %q", space.Key)
	}
	resolved := map[string]string{}
	for i := range metas {
		legacy := metas[i].Session
		canonical, ok := resolved[legacy]
		if !ok {
			if canonical, err = s.canonicalAuthor(ctx, legacy); err != nil {
				return nil, err
			}
			resolved[legacy] = canonical
		}
		metas[i].Session = canonical
	}
	return metas, nil
}

// canonicalAuthor maps a numeric chat id that a canvas file stored before
// chats moved to UUIDs onto the chat's UUID, so a pane matches the canvas to
// the chat that wrote it. A chat deleted since then keeps the old number.
func (s *DriveService) canonicalAuthor(ctx context.Context, session string) (string, error) {
	if legacyID, err := strconv.ParseInt(session, 10, 64); err != nil || legacyID <= 0 {
		return session, nil
	}
	rec, err := s.sessions.Get(ctx, session)
	if stores.IsNotFound(err) {
		return session, nil
	}
	if err != nil {
		return "", Wrap(err, KindInternal, "loading the chat that wrote a canvas")
	}
	return rec.ID, nil
}

func (s *DriveService) withCanonicalAuthor(ctx context.Context, c drive.Canvas) (drive.Canvas, error) {
	session, err := s.canonicalAuthor(ctx, c.Session)
	if err != nil {
		return drive.Canvas{}, err
	}
	c.Session = session
	return c, nil
}

// canvasGlobalSession is the session argument of an agent outside any chat
// or hive session. It still reads "global", not "shared", because existing
// agent configurations and the HIVE_AGENT_SESSION docs pass that word.
const canvasGlobalSession = "global"

func (s *DriveService) engineDrive() (*drivesvc.Service, error) {
	engine := s.engine()
	if engine == nil {
		return nil, Errorf(KindUnavailable, "the Hive runtime is not open")
	}
	return engine.Drive(), nil
}

func (s *DriveService) open(ctx context.Context, session string) (*drivesvc.Service, driveCaller, error) {
	d, err := s.engineDrive()
	if err != nil {
		return nil, driveCaller{}, err
	}
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return nil, driveCaller{}, err
	}
	return d, caller, nil
}

// resolve reads the tool's session argument. A UUID is a chat's record id;
// a positive integer is a migrated chat's legacy alias. HIVE_AGENT_SESSION
// carries one of those values. "global" is an agent outside any session.
// Anything else is the caller's working directory: hive puts no id in a
// session's environment, and an agent the user wired up by hand has only
// where it runs
// (ADR a-code-session-s-canvases-belong-to-its-repository-and-live-in-the-hive-context-directory).
//
// A directory inside no session stays not_found rather than falling back to
// the shared space: until the hive engine starts, every directory is inside
// no session, and a session's agent would file its canvases where its pane
// never looks (ADR agents-outside-every-session-share-one-global-canvas-owner).
func (s *DriveService) resolve(ctx context.Context, session string) (driveCaller, error) {
	session = strings.TrimSpace(session)
	if session == canvasGlobalSession {
		return driveCaller{space: drive.Shared}, nil
	}
	if !filepath.IsAbs(session) {
		if _, err := uuid.Parse(session); err != nil {
			legacyID, legacyErr := strconv.ParseInt(session, 10, 64)
			if legacyErr != nil || legacyID <= 0 {
				return driveCaller{}, Errorf(KindInvalid,
					"session %q is not a chat's HIVE_AGENT_SESSION, the absolute path of a working directory, or global", session)
			}
		}
		rec, err := s.sessions.Get(ctx, session)
		if stores.IsNotFound(err) {
			return driveCaller{}, Errorf(KindNotFound, "session %q not found", session)
		}
		if err != nil {
			return driveCaller{}, Wrap(err, KindInternal, "loading session %q", session)
		}
		space, err := drive.WorkspaceSpace(rec.Workspace)
		if err != nil {
			return driveCaller{}, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", rec.Workspace)
		}
		return driveCaller{space: space, author: drive.Author{Session: rec.ID}}, nil
	}
	engine := s.engine()
	if engine == nil {
		return driveCaller{}, Errorf(KindUnavailable, "hive sessions are not available in this build")
	}
	sess, ok, err := engine.Sessions().SessionAtPath(ctx, session)
	if err != nil {
		return driveCaller{}, Wrap(err, KindInternal, "finding the hive session at %q", session)
	}
	if !ok {
		return driveCaller{}, Errorf(KindNotFound,
			"no hive session's checkout holds %q; pass HIVE_AGENT_SESSION from a Hive chat, the working directory of a hive session, or global outside both", session)
	}
	space, ok := engine.Drive().SpaceForRemote(sess.Remote)
	if !ok {
		return driveCaller{}, Errorf(KindInvalid,
			"hive session %q has a remote with no owner and repository to file canvases under", sess.Name)
	}
	return driveCaller{space: space, author: drive.Author{HiveSession: sess.ID}}, nil
}

type driveTarget struct {
	svc   *drivesvc.Service
	space drive.Space
	path  string
}

func (s *DriveService) target(space, path string) (driveTarget, error) {
	d, err := s.engineDrive()
	if err != nil {
		return driveTarget{}, err
	}
	parsed, err := parseSpace(space)
	if err != nil {
		return driveTarget{}, err
	}
	docPath, err := canvasPath(path)
	if err != nil {
		return driveTarget{}, err
	}
	return driveTarget{svc: d, space: parsed, path: docPath}, nil
}

func parseSpace(key string) (drive.Space, error) {
	space, err := drive.ParseSpaceKey(key)
	if err != nil {
		return drive.Space{}, Errorf(KindInvalid, "%q is not a repository, workspace, or shared space", key)
	}
	return space, nil
}

// canvasPath maps a canvas name, or a document path, onto the document path
// in its space. A bad name is the caller's mistake, not an internal failure.
func canvasPath(name string) (string, error) {
	path, err := drive.CanvasPath(name)
	if err != nil {
		return "", invalidCanvasName(name)
	}
	return path, nil
}

func invalidCanvasName(name string) error {
	return Errorf(KindInvalid, "canvas name %q is not allowed: use a short lowercase name like release-notes (letters, digits, dots, hyphens, underscores)", name)
}

func canvasNotFound(name string) error {
	return Errorf(KindNotFound, "no canvas named %q in this workspace", name)
}

// driveError maps the drive's sentinel errors onto typed service errors, so
// a bad name or a missing canvas reads as the caller's mistake, not an
// internal failure.
func driveError(err error, name string) error {
	switch {
	case errors.Is(err, drive.ErrNotFound):
		return canvasNotFound(name)
	case errors.Is(err, drive.ErrInvalidPath):
		return invalidCanvasName(name)
	case errors.Is(err, drive.ErrInvalidSpace):
		return Errorf(KindInvalid, "workspace is not a valid workspace directory name")
	case errors.Is(err, drive.ErrExists), errors.Is(err, drive.ErrNotEmpty):
		return Wrap(err, KindConflict, "canvas %q", name)
	case errors.Is(err, drive.ErrNotWritable):
		return Wrap(err, KindInvalid, "canvas %q", name)
	default:
		return Wrap(err, KindInternal, "canvas %q", name)
	}
}

// plainTitle decodes HTML entities. A title is plain text on every surface,
// and agents often escape one as if it were markup, so "&amp;" would show
// as written.
func plainTitle(title string) string {
	return html.UnescapeString(title)
}

func validateBlock(b *drive.Block) error {
	b.ID = strings.TrimSpace(b.ID)
	b.Title = plainTitle(b.Title)
	if b.ID == "" {
		return Errorf(KindInvalid, "a block needs an id")
	}
	if len(b.ID) > maxCanvasBlockIDLength {
		return Errorf(KindInvalid, "block id is too long (%d chars max)", maxCanvasBlockIDLength)
	}
	switch b.Kind {
	case drive.BlockKindMarkdown:
		if b.Body == "" {
			return Errorf(KindInvalid, "a markdown block needs a body")
		}
		if len(b.Body) > maxCanvasBodyBytes {
			return Errorf(KindInvalid, "block body is too large (%d bytes max)", maxCanvasBodyBytes)
		}
		if b.URL != "" {
			return Errorf(KindInvalid, "a markdown block carries no url; use a link block")
		}
	case drive.BlockKindHTML:
		if b.Body == "" {
			return Errorf(KindInvalid, "an html block needs a body")
		}
		if len(b.Body) > maxCanvasBodyBytes {
			return Errorf(KindInvalid, "block body is too large (%d bytes max)", maxCanvasBodyBytes)
		}
		if b.URL != "" {
			return Errorf(KindInvalid, "an html block carries no url; use a link block")
		}
		// The sanitizer drops what it does not know, and a silent drop is
		// the one failure an agent cannot see: refuse the write and name the
		// offender instead of rendering a layout it believes is intact.
		if rejected := canvas.RejectedHTML(b.Body); rejected != "" {
			return Errorf(KindInvalid,
				"an html block cannot use %s; see the hive-drive docs for the tags and attributes it accepts, and link only to http, https or mailto",
				rejected)
		}
	case drive.BlockKindLink:
		if b.Title == "" {
			return Errorf(KindInvalid, "a link block needs a title")
		}
		if b.Body != "" {
			return Errorf(KindInvalid, "a link block carries no body; use a markdown block")
		}
		if err := validateLinkURL(b.URL); err != nil {
			return err
		}
	default:
		return Errorf(KindInvalid, "unknown block kind %q; use %q, %q or %q", b.Kind, drive.BlockKindMarkdown, drive.BlockKindHTML, drive.BlockKindLink)
	}
	return nil
}

// validateLinkURL mirrors the frontend's link filter: only schemes the pane
// will open are accepted, so a block never renders a link the click handler
// then refuses.
func validateLinkURL(raw string) error {
	if raw == "" {
		return Errorf(KindInvalid, "a link block needs a url")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return Errorf(KindInvalid, "link url %q does not parse", raw)
	}
	switch parsed.Scheme {
	case "http", "https", "mailto":
		return nil
	default:
		return Errorf(KindInvalid, "link url scheme %q is not allowed; use http, https or mailto", parsed.Scheme)
	}
}

// legacyCanvasImport is one canvases directory the desktop filed outside the
// drive before it existed, and the space it belongs in now.
type legacyCanvasImport struct {
	from  string
	space drive.Space
}

// legacyCanvasImports names the shared canvases directory under the desktop
// data dir and one per workspace directory. Only the desktop knows these
// roots, which is why the engine's import runs from here. skipped holds a
// workspace entry that could not be read or named as a space; a plain file
// beside the workspaces is not one.
func legacyCanvasImports(dataDir, workspacesDir string) (imports []legacyCanvasImport, skipped []error, err error) {
	imports = []legacyCanvasImport{{from: filepath.Join(dataDir, drive.CanvasesDir), space: drive.Shared}}
	entries, err := os.ReadDir(workspacesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return imports, nil, nil
	}
	if err != nil {
		return imports, nil, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(workspacesDir, name))
		if err != nil {
			skipped = append(skipped, fmt.Errorf("workspace %q: %w", name, err))
			continue
		}
		if !info.IsDir() {
			continue
		}
		space, err := drive.WorkspaceSpace(name)
		if err != nil {
			skipped = append(skipped, err)
			continue
		}
		imports = append(imports, legacyCanvasImport{from: filepath.Join(workspacesDir, name, drive.CanvasesDir), space: space})
	}
	return imports, skipped, nil
}

// importLegacyCanvases moves every pre-drive canvas into its space. Each
// import is idempotent and a failure is logged rather than fatal: a canvas
// left where it was is still a file the user can move by hand.
func importLegacyCanvases(ctx context.Context, logger zerolog.Logger, engine *hive.Engine, paths settings.Paths) {
	imports, skipped, err := legacyCanvasImports(paths.DataDir, paths.AgentWorkspacesDir)
	if err != nil {
		logger.Warn().Err(err).Str("dir", paths.AgentWorkspacesDir).Msg("workspace canvases not listed for import")
	}
	for _, err := range skipped {
		logger.Warn().Err(err).Str("dir", paths.AgentWorkspacesDir).Msg("workspace skipped for legacy canvas import")
	}
	for _, imp := range imports {
		moved, err := engine.Drive().ImportLegacyCanvases(ctx, imp.from, imp.space)
		if err != nil {
			logger.Warn().Err(err).Str("from", imp.from).Str("space", imp.space.Key).Msg("legacy canvases not imported")
			continue
		}
		if moved > 0 {
			logger.Info().Int("count", moved).Str("from", imp.from).Str("space", imp.space.Key).Msg("imported legacy canvases into the drive")
		}
	}
}
