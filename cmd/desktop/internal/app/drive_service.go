package app

import (
	"context"
	"errors"
	"html"
	"io/fs"
	"net/url"
	"os"
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
	"github.com/colonyops/hive/internal/platform/git"
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

// DriveService is the desktop's door onto Hive Drive. Agents mutate canvas
// blocks and front matter through hive-canvas; the pane reads by space key
// and may delete a whole canvas.
type DriveService struct {
	// engine is read per call: it starts after the services are built, and
	// every method answers KindUnavailable until it has.
	engine   func() *hive.Engine
	sessions canvasSessionResolver
	events   *events.Bus
}

type DriveDeps struct {
	Engine   func() *hive.Engine
	Sessions canvasSessionResolver
	Events   *events.Bus
}

func newDriveService(d DriveDeps) *DriveService {
	return &DriveService{engine: d.Engine, sessions: d.Sessions, events: d.Events}
}

// driveCaller is who a tool call comes from and the space its canvases
// belong to.
type driveCaller struct {
	space  drive.Space
	author drive.Author
}

// legacyGlobalOwner is the owner key the frontend still addresses the shared
// space by; the @ kept it out of the workspace names hive creates.
const legacyGlobalOwner = "@global"

// SpaceKeyForOwner maps the owner key the frontend addresses a canvas by onto
// a drive space key: @global is the shared space, a slash-less owner is a
// workspace, and owner/repo already is one.
func SpaceKeyForOwner(owner string) string {
	switch {
	case owner == legacyGlobalOwner:
		return drive.SharedKey
	case !strings.Contains(owner, "/"):
		return drive.WorkspacesDir + "/" + owner
	default:
		return owner
	}
}

// OwnerForSpaceKey is the inverse of SpaceKeyForOwner.
func OwnerForSpaceKey(key string) string {
	if key == drive.SharedKey {
		return legacyGlobalOwner
	}
	if name, ok := strings.CutPrefix(key, drive.WorkspacesDir+"/"); ok {
		return name
	}
	return key
}

// SpaceKeyForRemote is the space key of the repository a hive session
// checked out, or "" for a remote that names no owner and repo. A function
// rather than a method because the Wails session wire has no engine handle.
func SpaceKeyForRemote(remote string) string {
	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return ""
	}
	space, err := drive.RepositorySpace(owner, repo)
	if err != nil {
		return ""
	}
	return space.Key
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
	s.notify(ctx, caller.author)
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
	c, removed, err := d.RemoveBlock(ctx, caller.space, path, blockID)
	if err != nil {
		return drive.Canvas{}, driveError(err, name)
	}
	if !removed {
		return drive.Canvas{}, Errorf(KindNotFound, "no block %q on canvas %q", blockID, name)
	}
	s.notify(ctx, caller.author)
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
	c, err := d.SetCanvasFrontmatter(ctx, caller.space, path, fm)
	if err != nil {
		return drive.Canvas{}, driveError(err, name)
	}
	s.notify(ctx, caller.author)
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
	c, err := d.ClearCanvas(ctx, caller.space, path)
	if err != nil {
		return drive.Canvas{}, driveError(err, name)
	}
	s.notify(ctx, caller.author)
	return s.withCanonicalAuthor(ctx, c)
}

// DeleteCanvas removes one canvas file entirely for an agent caller.
func (s *DriveService) DeleteCanvas(ctx context.Context, session, name string) error {
	d, caller, err := s.open(ctx, session)
	if err != nil {
		return err
	}
	path, err := canvasPath(name)
	if err != nil {
		return err
	}
	if err := d.Delete(ctx, caller.space, path); err != nil {
		return driveError(err, name)
	}
	s.notify(ctx, caller.author)
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
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return err
	}
	if caller.space.Kind == drive.SpaceKindShared {
		return Errorf(KindInvalid, "a global canvas has no pane to open or close; the user reads it in the full-page canvas view")
	}
	pinned := ""
	if open && name != "" {
		c, err := s.GetCanvas(ctx, session, name)
		if err != nil {
			return err
		}
		pinned = drive.CanvasName(c.Path)
	}
	s.events.Publish(ctx, events.CanvasToggleRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession, Name: pinned, Open: open,
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

// ListCanvasesAt is a space's canvases, most recently updated first. A
// canvas whose creating chat or session is gone still lists: the artifact
// outlives what produced it.
func (s *DriveService) ListCanvasesAt(ctx context.Context, space string) ([]drive.CanvasMeta, error) {
	d, err := s.engineDrive()
	if err != nil {
		return nil, err
	}
	parsed, err := parseSpace(space)
	if err != nil {
		return nil, err
	}
	return s.listCanvases(ctx, d, parsed)
}

// DeleteAt removes one canvas for the UI and publishes with the author that
// wrote it, so the pane beside that chat or session wakes.
func (s *DriveService) DeleteAt(ctx context.Context, space, path string) error {
	t, err := s.target(space, path)
	if err != nil {
		return err
	}
	c, ok, err := t.svc.LoadCanvas(ctx, t.space, t.path)
	if err != nil {
		return driveError(err, path)
	}
	if !ok {
		return canvasNotFound(path)
	}
	if err := t.svc.Delete(ctx, t.space, t.path); err != nil {
		return driveError(err, path)
	}
	s.notify(ctx, drive.Author{Session: c.Session, HiveSession: c.HiveSession})
	return nil
}

// Markdown renders one canvas as a standalone markdown document, the pane's
// copy action. A canvas nothing was written under is not_found: exporting
// nothing is a mistake worth surfacing, unlike showing it.
func (s *DriveService) Markdown(ctx context.Context, space, path string) (string, error) {
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

// Export writes one canvas's markdown rendering to dest, the pane's save
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
		return Wrap(err, KindInternal, "writing export for canvas %q", path)
	}
	return nil
}

// Repositories is the space key of every repository in the drive, so a
// reader can offer them beside the workspaces.
func (s *DriveService) Repositories(ctx context.Context) ([]string, error) {
	d, err := s.engineDrive()
	if err != nil {
		return nil, err
	}
	spaces, err := d.Spaces(ctx)
	if err != nil {
		return nil, Wrap(err, KindInternal, "listing repositories in the drive")
	}
	keys := []string{}
	for _, space := range spaces {
		if space.Kind == drive.SpaceKindRepository {
			keys = append(keys, space.Key)
		}
	}
	return keys, nil
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
// or hive session.
const canvasGlobalSession = "global"

func (s *DriveService) engineDrive() (*drivesvc.Service, error) {
	engine := s.engine()
	if engine == nil {
		return nil, Errorf(KindUnavailable, "the Hive runtime is not open")
	}
	return engine.Drive(), nil
}

// open is the drive and the caller every agent operation starts from.
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

// driveTarget is a UI call's canvas: the drive, the space it named, and the
// document path its name or path maps to.
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

func (s *DriveService) notify(ctx context.Context, author drive.Author) {
	s.events.Publish(ctx, events.CanvasUpdated{Session: author.Session, HiveSession: author.HiveSession})
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
				"an html block cannot use %s; see the hive-canvas docs for the tags and attributes it accepts, and link only to http, https or mailto",
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
// roots, which is why the engine's import runs from here.
func legacyCanvasImports(dataDir, workspacesDir string) ([]legacyCanvasImport, error) {
	imports := []legacyCanvasImport{{from: filepath.Join(dataDir, drive.CanvasesDir), space: drive.Shared}}
	entries, err := os.ReadDir(workspacesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return imports, nil
	}
	if err != nil {
		return imports, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if info, err := os.Stat(filepath.Join(workspacesDir, name)); err != nil || !info.IsDir() {
			continue
		}
		space, err := drive.WorkspaceSpace(name)
		if err != nil {
			continue
		}
		imports = append(imports, legacyCanvasImport{from: filepath.Join(workspacesDir, name, drive.CanvasesDir), space: space})
	}
	return imports, nil
}

// importLegacyCanvases moves every pre-drive canvas into its space. Each
// import is idempotent and a failure is logged rather than fatal: a canvas
// left where it was is still a file the user can move by hand.
func importLegacyCanvases(ctx context.Context, logger zerolog.Logger, engine *hive.Engine, paths settings.Paths) {
	imports, err := legacyCanvasImports(paths.DataDir, paths.AgentWorkspacesDir)
	if err != nil {
		logger.Warn().Err(err).Str("dir", paths.AgentWorkspacesDir).Msg("workspace canvases not listed for import")
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
