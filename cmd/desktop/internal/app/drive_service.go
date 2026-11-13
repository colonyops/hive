package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/cmd/desktop/internal/app/htmldoc"
	"github.com/colonyops/hive/internal/domain/drive"
	hivesession "github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/hive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
	sessionsvc "github.com/colonyops/hive/internal/hive/session"
	"github.com/colonyops/hive/internal/ops"
	"github.com/colonyops/hive/pkg/logutils"
	"github.com/colonyops/hive/pkg/osopen"
)

// chatSessionResolver is the one store read a chat's tool call starts with:
// the session record is the authority on which workspace a document belongs
// to, so an agent never names the workspace itself.
type chatSessionResolver interface {
	Get(ctx context.Context, id string) (stores.AgentSession, error)
}

// DriveService is the desktop's door onto Hive Drive. Agents read and write
// documents through the hive-drive MCP tools; the UI reads
// and reorganizes a space by its key.
type DriveService struct {
	// engine is read per call: it starts after the services are built, and
	// every method answers KindUnavailable until it has.
	engine   func() *hive.Engine
	sessions chatSessionResolver
	events   *events.Bus
	reveal   func(string) error
	logger   zerolog.Logger
}

type DriveDeps struct {
	Engine   func() *hive.Engine
	Sessions chatSessionResolver
	Events   *events.Bus
	Logger   zerolog.Logger
	// Reveal shows an absolute path in the file manager; nil is osopen.Reveal.
	Reveal func(string) error
}

func newDriveService(d DriveDeps) *DriveService {
	if d.Reveal == nil {
		d.Reveal = osopen.Reveal
	}
	return &DriveService{engine: d.Engine, sessions: d.Sessions, events: d.Events, reveal: d.Reveal, logger: logutils.Component(d.Logger, "desktop.drive")}
}

// driveCaller is who a tool call comes from and the space its documents
// belong to.
type driveCaller struct {
	space  drive.Space
	author drive.Author
	// checkout and tmuxSession are set for a hive session only: its working
	// tree and the name of its tmux session.
	checkout    string
	tmuxSession string
}

// Markdown is a markdown document's own body, the reader's copy action. Any
// other kind has no markdown form and is refused here, so the policy does not
// live only in the frontend.
func (s *DriveService) Markdown(ctx context.Context, space, path string) (string, error) {
	d, parsed, err := s.space(space)
	if err != nil {
		return "", err
	}
	if drive.KindOf(path) != drive.KindMarkdown {
		return "", Errorf(KindInvalid, "%q is not a markdown document; only markdown documents export as markdown", path)
	}
	_, body, err := d.Read(ctx, parsed, path)
	if err != nil {
		return "", documentError(err, parsed, path)
	}
	return string(body), nil
}

// Export writes one document's Markdown to dest, the Drive tab's save
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
// Markdown and HTML is set for those kinds. A plain file carries Image when it
// is a drawable image, Text when it is text, and otherwise only its entry.
type DocumentView struct {
	Space    drive.Space `json:"space"`
	Entry    drive.Entry `json:"entry"`
	Markdown string      `json:"markdown,omitempty"` // raw: the frontend's sanitizing renderer handles it
	HTML     string      `json:"html,omitempty"`     // CSP meta injected first
	Image    string      `json:"image,omitempty"`    // data URL; empty for a file that is not a drawable image
	Text     string      `json:"text,omitempty"`
}

// maxTextBytes caps the text file a document carries. The reader virtualizes
// what it draws, but the body still crosses the binding whole, and a larger
// file is rarely one a person reads in a viewer.
const maxTextBytes = 4 << 20

// isText reports whether body reads as text: valid UTF-8 with no NUL byte,
// which binary formats almost always hold early on.
func isText(body []byte) bool {
	return utf8.Valid(body) && !bytes.Contains(body, []byte{0})
}

// maxImageBytes caps the image a document carries. It crosses the binding whole
// as base64, so a larger file stays a plain file the user opens elsewhere.
const maxImageBytes = 32 << 20

var imageMediaTypes = map[string]string{
	".avif": "image/avif",
	".gif":  "image/gif",
	".ico":  "image/x-icon",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
}

func imageMediaType(entry drive.Entry) string {
	if entry.Size > maxImageBytes {
		return ""
	}
	return imageMediaTypes[strings.ToLower(pathpkg.Ext(entry.Path))]
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

// Recent is every space's documents, newest created first; limit 0 is the
// drive's default cap.
func (s *DriveService) Recent(ctx context.Context, limit int) ([]drivesvc.Hit, error) {
	d, err := s.engineDrive()
	if err != nil {
		return nil, err
	}
	if limit < 0 {
		return nil, Errorf(KindInvalid, "recent limit must not be negative")
	}
	hits, err := d.Recent(ctx, limit)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing recent drive documents")
	}
	return hits, nil
}

// Document is one document read for the reader. The kind comes from the
// name, and a plain file is read only when its size allows it to be an image
// or text the reader shows.
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
	case drive.KindFile:
		entry, err := d.Entry(ctx, parsed, path)
		if err != nil {
			return DocumentView{}, documentError(err, parsed, path)
		}
		if entry.Kind == drive.KindDir {
			return DocumentView{}, Errorf(KindInvalid, "%q is a folder, not a document", path)
		}
		view.Entry = entry
		if mediaType := imageMediaType(entry); mediaType != "" {
			_, body, err := d.Read(ctx, parsed, path)
			if err != nil {
				return DocumentView{}, documentError(err, parsed, path)
			}
			view.Image = "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(body)
			return view, nil
		}
		if entry.Size <= maxTextBytes {
			_, body, err := d.Read(ctx, parsed, path)
			if err != nil {
				return DocumentView{}, documentError(err, parsed, path)
			}
			if isText(body) {
				view.Text = string(body)
			}
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
		view.HTML = htmldoc.WrapDocument(body)
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

// Delete removes a document or an empty folder.
func (s *DriveService) Delete(ctx context.Context, space, path string) error {
	d, parsed, err := s.space(space)
	if err != nil {
		return err
	}
	if err := documentPath(path); err != nil {
		return err
	}
	if err := d.Delete(ctx, parsed, path, drive.Author{}); err != nil {
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
		return name + pathpkg.Ext(baseDrivePath(path))
	case got == drive.KindFile && pathpkg.Ext(name) == "":
		return name + pathpkg.Ext(path)
	}
	return name
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
// such as keeping the extension. A
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
// addresses a document by space and path.
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

func (s *DriveService) tabCaller(ctx context.Context, session string) (driveCaller, error) {
	_, caller, err := s.open(ctx, session)
	return caller, err
}

// resolveTabPath checks that path is a document in the caller's space, the
// one the Drive tab shows.
func (s *DriveService) resolveTabPath(_ context.Context, caller driveCaller, path string) (string, error) {
	d, err := s.engineDrive()
	if err != nil {
		return "", err
	}
	space := caller.space
	if err := drive.ValidatePath(path); err != nil {
		return "", documentError(err, space, path)
	}
	isDir, err := s.isDir(d, space, path)
	switch {
	case err == nil && isDir:
		return "", Errorf(KindInvalid, "%q is a folder; open a document inside it", path)
	case err != nil:
		return "", err
	}
	return path, nil
}

// resolve reads the tool's caller argument through the engine's ResolveCaller.
// A chat is the one kind the engine cannot look up: its record lives in the
// desktop's store.
//
// A directory inside no session stays not_found rather than falling back to
// the shared space: until the hive engine starts, every directory is inside
// no session, and a session's agent would file its documents where its Drive tab
// never looks (ADR agents-outside-every-session-share-one-global-canvas-owner).
func (s *DriveService) resolve(ctx context.Context, session string) (driveCaller, error) {
	engine := s.engine()
	if engine == nil {
		// A chat and global need no hive engine, so they still resolve before it runs.
		value := strings.TrimSpace(session)
		switch {
		case value == sessionsvc.CallerGlobal:
			return driveCaller{space: drive.Shared}, nil
		case sessionsvc.LooksLikeChatID(value):
			return s.chatCaller(ctx, value)
		}
		return driveCaller{}, Errorf(KindUnavailable, "hive sessions are not available in this build")
	}
	caller, err := engine.Sessions().ResolveCaller(ctx, session, "", "")
	if err != nil {
		if opErr, ok := errors.AsType[*ops.Error](err); ok {
			return driveCaller{}, Errorf(Kind(opErr.Kind), "%s", opErr.Msg)
		}
		return driveCaller{}, Wrap(err, KindInternal, "resolving caller %q", session)
	}
	switch caller.Kind {
	case sessionsvc.CallerKindGlobal:
		return driveCaller{space: drive.Shared}, nil
	case sessionsvc.CallerKindChat:
		return s.chatCaller(ctx, caller.ChatID)
	case sessionsvc.CallerKindSession:
		return hiveSessionCaller(engine, caller.Session)
	}
	return driveCaller{}, Errorf(KindInternal, "caller %q resolved to an unknown kind %q", session, caller.Kind)
}

func hiveSessionCaller(engine *hive.Engine, sess hivesession.Session) (driveCaller, error) {
	space, ok := engine.Drive().SpaceForRemote(sess.Remote)
	if !ok {
		return driveCaller{}, Errorf(KindInvalid,
			"hive session %q has a remote with no owner and repository to file documents under", sess.Name)
	}
	return driveCaller{
		space: space, author: drive.Author{HiveSession: sess.ID},
		checkout: sess.Path, tmuxSession: sessionsvc.Target(sess).Session,
	}, nil
}

func (s *DriveService) chatCaller(ctx context.Context, chatID string) (driveCaller, error) {
	rec, err := s.sessions.Get(ctx, chatID)
	if stores.IsNotFound(err) {
		return driveCaller{}, Errorf(KindNotFound, "session %q not found", chatID)
	}
	if err != nil {
		return driveCaller{}, Wrap(err, KindInternal, "loading session %q", chatID)
	}
	space, err := drive.WorkspaceSpace(rec.Workspace)
	if err != nil {
		return driveCaller{}, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", rec.Workspace)
	}
	return driveCaller{space: space, author: drive.Author{Session: rec.ID}}, nil
}

func parseSpace(key string) (drive.Space, error) {
	space, err := drive.ParseSpaceKey(key)
	if err != nil {
		return drive.Space{}, Errorf(KindInvalid, "%q is not a repository, workspace, or shared space", key)
	}
	return space, nil
}
