package app

import (
	"context"
	"errors"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/canvas"
	"github.com/colonyops/hive/cmd/desktop/internal/app/data/stores"
	"github.com/colonyops/hive/cmd/desktop/internal/app/events"
	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/platform/git"
)

// maxCanvasBodyBytes caps one markdown or html block's body so a single tool
// call cannot make the pane unrenderable.
const maxCanvasBodyBytes = 256 * 1024

const (
	maxCanvasBlockIDLength = 200
	maxCanvasTitleLength   = 200
)

// canvasSessionResolver is the one store read a chat's canvas mutation starts
// with: the session record is the authority on which workspace a canvas
// belongs to, so an agent never names the workspace itself.
type canvasSessionResolver interface {
	Get(ctx context.Context, id int64) (stores.AgentSession, error)
}

// canvasHiveSessions is the same authority for an agent in a hive session:
// the session whose checkout holds the agent's working directory decides the
// repository, so the agent never names that either.
type canvasHiveSessions interface {
	SessionAtPath(ctx context.Context, path string) (session.Session, bool, error)
}

// CanvasService is an owner's canvases: named, agent-written artifacts shown
// beside a chat or a Code session. Writes arrive only through the hive-canvas
// MCP tools; the frontend reads
// (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry).
type CanvasService struct {
	store        *canvas.Store
	sessions     canvasSessionResolver
	hiveSessions canvasHiveSessions
	events       *events.Bus
}

type CanvasDeps struct {
	Store        *canvas.Store
	Sessions     canvasSessionResolver
	HiveSessions canvasHiveSessions
	Events       *events.Bus
}

func newCanvasService(d CanvasDeps) *CanvasService {
	return &CanvasService{store: d.Store, sessions: d.Sessions, hiveSessions: d.HiveSessions, events: d.Events}
}

// canvasCaller is who a tool call comes from and the owner its canvases
// belong to.
type canvasCaller struct {
	owner  string
	author canvas.Author
}

// CanvasOwnerForRemote is the owner key of the repository a hive session
// checked out, or "" for a remote that names no owner and repo. It is the
// derivation hive's context directory uses, so a repository's canvases sit in
// the directory its checkouts link as .hive.
func CanvasOwnerForRemote(remote string) string {
	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return ""
	}
	return canvas.RepositoryOwner(owner, repo)
}

// Get returns one canvas in the calling session's workspace. A name nothing
// was ever written under is not_found — unlike the pane, an agent asking for
// a canvas by name should learn the name is wrong, not see a blank surface.
func (s *CanvasService) Get(ctx context.Context, session string, name string) (canvas.Canvas, error) {
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return canvas.Canvas{}, err
	}
	c, ok, err := s.store.Load(caller.owner, name)
	if err != nil {
		return canvas.Canvas{}, s.storeError(err, name)
	}
	if !ok {
		return canvas.Canvas{}, Errorf(KindNotFound, "no canvas named %q in this workspace", name)
	}
	return c, nil
}

// PutBlock creates or replaces one block, creating the canvas on its first
// write. A non-empty title renames the canvas; empty leaves the stored one.
// A non-empty before places the block ahead of that existing block id
// instead of appending. Returns the canvas after the write.
func (s *CanvasService) PutBlock(ctx context.Context, session string, name, title, before string, b canvas.Block) (canvas.Canvas, error) {
	return s.putBlocks(ctx, session, name, title, before, []canvas.Block{b})
}

// maxCanvasBatchBlocks caps one put_blocks call; a layout larger than this
// is written in slices.
const maxCanvasBatchBlocks = 50

// PutBlocks writes a batch of blocks as one atomic canvas write — one file
// write, one pane render. Every block is validated before any is written, so
// a rejected batch leaves the canvas untouched.
func (s *CanvasService) PutBlocks(ctx context.Context, session string, name, title string, blocks []canvas.Block) (canvas.Canvas, error) {
	if len(blocks) == 0 {
		return canvas.Canvas{}, Errorf(KindInvalid, "a batch needs at least one block")
	}
	if len(blocks) > maxCanvasBatchBlocks {
		return canvas.Canvas{}, Errorf(KindInvalid, "too many blocks in one batch (%d max)", maxCanvasBatchBlocks)
	}
	return s.putBlocks(ctx, session, name, title, "", blocks)
}

func (s *CanvasService) putBlocks(ctx context.Context, session string, name, title, before string, blocks []canvas.Block) (canvas.Canvas, error) {
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return canvas.Canvas{}, err
	}
	title = plainTitle(title)
	if len(title) > maxCanvasTitleLength {
		return canvas.Canvas{}, Errorf(KindInvalid, "canvas title is too long (%d chars max)", maxCanvasTitleLength)
	}
	seen := make(map[string]bool, len(blocks))
	for i := range blocks {
		if err := validateBlock(&blocks[i]); err != nil {
			if len(blocks) > 1 {
				return canvas.Canvas{}, Wrap(err, KindInvalid, "blocks[%d]", i)
			}
			return canvas.Canvas{}, err
		}
		if seen[blocks[i].ID] {
			return canvas.Canvas{}, Errorf(KindInvalid, "block id %q appears twice in one batch", blocks[i].ID)
		}
		seen[blocks[i].ID] = true
	}
	c, err := s.store.Upsert(caller.owner, name, caller.author, title, before, blocks...)
	if err != nil {
		if errors.Is(err, canvas.ErrAnchorNotFound) {
			return canvas.Canvas{}, Errorf(KindNotFound, "no block %q on canvas %q to place before", before, name)
		}
		return canvas.Canvas{}, s.storeError(err, name)
	}
	s.notify(ctx, caller.author)
	return c, nil
}

// RemoveBlock deletes one block by id and returns the canvas that remains.
func (s *CanvasService) RemoveBlock(ctx context.Context, session string, name, blockID string) (canvas.Canvas, error) {
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return canvas.Canvas{}, err
	}
	c, removed, err := s.store.Remove(caller.owner, name, blockID)
	if err != nil {
		return canvas.Canvas{}, s.storeError(err, name)
	}
	if !removed {
		return canvas.Canvas{}, Errorf(KindNotFound, "no block %q on canvas %q", blockID, name)
	}
	s.notify(ctx, caller.author)
	return c, nil
}

// Clear removes every block at once; the canvas, its title and its file
// survive.
func (s *CanvasService) Clear(ctx context.Context, session string, name string) (canvas.Canvas, error) {
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return canvas.Canvas{}, err
	}
	c, err := s.store.Clear(caller.owner, name)
	if err != nil {
		return canvas.Canvas{}, s.storeError(err, name)
	}
	s.notify(ctx, caller.author)
	return c, nil
}

// SetPaneOpen asks the UI to open or close the canvas pane beside the
// calling chat or Code session — UI intent, applied only while the user is
// viewing it, so an agent can surface what it made without ever dragging the
// user away from something else. Opening with a name requires that canvas to
// exist; empty leaves the pane's own pick.
func (s *CanvasService) SetPaneOpen(ctx context.Context, session string, name string, open bool) error {
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return err
	}
	if caller.owner == canvas.GlobalOwner {
		return Errorf(KindInvalid, "a global canvas has no pane to open or close; the user reads it in the full-page canvas view")
	}
	if open && name != "" {
		if _, err := s.Get(ctx, session, name); err != nil {
			return err
		}
	}
	s.events.Publish(ctx, events.CanvasToggleRequested{
		Session: caller.author.Session, HiveSession: caller.author.HiveSession, Name: name, Open: open,
	})
	return nil
}

// Delete removes one canvas file entirely.
func (s *CanvasService) Delete(ctx context.Context, session string, name string) error {
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return err
	}
	existed, err := s.store.Delete(caller.owner, name)
	if err != nil {
		return s.storeError(err, name)
	}
	if !existed {
		return Errorf(KindNotFound, "no canvas named %q in this workspace", name)
	}
	s.notify(ctx, caller.author)
	return nil
}

// List returns the calling session's workspace canvases, most recently
// updated first.
func (s *CanvasService) List(ctx context.Context, session string) ([]canvas.Meta, error) {
	caller, err := s.resolve(ctx, session)
	if err != nil {
		return nil, err
	}
	return s.ListForOwner(ctx, caller.owner)
}

// GetForOwner is the pane's read: addressed by owner key, and a name
// nothing was written under answers an empty canvas rather than an error, so
// the pane never flashes a failure for a canvas that was deleted under it.
// html blocks leave here sanitized — this is the one seam between stored
// agent source and the app's own webview, so the frontend never has to hold
// a policy of its own
// (ADR canvas-html-blocks-are-sanitized-in-go-and-styled-by-an-app-owned-class-vocabulary).
func (s *CanvasService) GetForOwner(_ context.Context, dir, name string) (canvas.Canvas, error) {
	c, ok, err := s.store.Load(dir, name)
	if err != nil {
		return canvas.Canvas{}, s.storeError(err, name)
	}
	if !ok {
		return canvas.Canvas{Workspace: dir, Name: name, Blocks: []canvas.Block{}}, nil
	}
	for i, b := range c.Blocks {
		if b.Kind == canvas.KindHTML {
			c.Blocks[i].Body = canvas.SanitizeHTML(b.Body)
		}
	}
	return c, nil
}

// MarkdownForOwner renders one canvas as a standalone markdown document
// — the pane's copy action. A name nothing was written under is not_found:
// exporting nothing is a mistake worth surfacing, unlike showing it.
func (s *CanvasService) MarkdownForOwner(_ context.Context, dir, name string) (string, error) {
	c, ok, err := s.store.Load(dir, name)
	if err != nil {
		return "", s.storeError(err, name)
	}
	if !ok {
		return "", Errorf(KindNotFound, "no canvas named %q in this workspace", name)
	}
	return canvas.Markdown(c), nil
}

// ExportForOwner writes one canvas's markdown rendering to path — the
// pane's save action, with path coming from the native save dialog, which is
// why it must already be absolute.
func (s *CanvasService) ExportForOwner(ctx context.Context, dir, name, path string) error {
	markdown, err := s.MarkdownForOwner(ctx, dir, name)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return Errorf(KindInvalid, "export path must be absolute")
	}
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		return Wrap(err, KindInternal, "writing export for canvas %q", name)
	}
	return nil
}

// ListForOwner returns an owner's canvas metadata, most recently updated
// first. A canvas whose creating chat or session is gone still lists — the
// artifact outlives what produced it.
func (s *CanvasService) ListForOwner(_ context.Context, dir string) ([]canvas.Meta, error) {
	metas, err := s.store.List(dir)
	if err != nil {
		if errors.Is(err, canvas.ErrInvalidWorkspace) {
			return nil, Errorf(KindInvalid, "workspace %q is not a valid workspace directory name", dir)
		}
		return nil, Wrap(err, KindInternal, "listing canvases for workspace %q", dir)
	}
	return metas, nil
}

// Repositories returns the owner key of every repository that holds a
// canvas, so a reader can offer them beside the workspaces.
func (s *CanvasService) Repositories(_ context.Context) ([]string, error) {
	keys, err := s.store.Repositories()
	return keys, Wrap(err, KindInternal, "listing repositories with canvases")
}

// canvasGlobalSession is the session argument of an agent outside any chat
// or hive session.
const canvasGlobalSession = "global"

// resolve reads the tool's session argument. A number is a chat's record id,
// what HIVE_AGENT_SESSION carries. "global" is an agent outside any session.
// Anything else is the caller's working directory: hive puts no id in a
// session's environment, and an agent the user wired up by hand has only
// where it runs
// (ADR a-code-session-s-canvases-belong-to-its-repository-and-live-in-the-hive-context-directory).
//
// A directory inside no session stays not_found rather than falling back to
// global: until the hive engine starts, every directory is inside no session,
// and a session's agent would file its canvases where its pane never looks
// (ADR agents-outside-every-session-share-one-global-canvas-owner).
func (s *CanvasService) resolve(ctx context.Context, session string) (canvasCaller, error) {
	session = strings.TrimSpace(session)
	if session == canvasGlobalSession {
		return canvasCaller{owner: canvas.GlobalOwner}, nil
	}
	if id, err := strconv.ParseInt(session, 10, 64); err == nil {
		rec, err := s.sessions.Get(ctx, id)
		if stores.IsNotFound(err) {
			return canvasCaller{}, Errorf(KindNotFound, "session %d not found", id)
		}
		if err != nil {
			return canvasCaller{}, Wrap(err, KindInternal, "loading session %d", id)
		}
		return canvasCaller{owner: rec.Workspace, author: canvas.Author{Session: id}}, nil
	}
	if !filepath.IsAbs(session) {
		return canvasCaller{}, Errorf(KindInvalid,
			"session %q is not a chat's HIVE_AGENT_SESSION, the absolute path of a working directory, or global", session)
	}
	if s.hiveSessions == nil {
		return canvasCaller{}, Errorf(KindUnavailable, "hive sessions are not available in this build")
	}
	sess, ok, err := s.hiveSessions.SessionAtPath(ctx, session)
	if err != nil {
		return canvasCaller{}, Wrap(err, KindInternal, "finding the hive session at %q", session)
	}
	if !ok {
		return canvasCaller{}, Errorf(KindNotFound,
			"no hive session's checkout holds %q; pass HIVE_AGENT_SESSION from a Hive chat, the working directory of a hive session, or global outside both", session)
	}
	owner := CanvasOwnerForRemote(sess.Remote)
	if owner == "" {
		return canvasCaller{}, Errorf(KindInvalid,
			"hive session %q has a remote with no owner and repository to file canvases under", sess.Name)
	}
	return canvasCaller{owner: owner, author: canvas.Author{HiveSession: sess.ID}}, nil
}

// storeError maps the store's sentinel errors onto typed service errors, so
// a bad name or a missing canvas reads as the caller's mistake, not an
// internal failure.
func (s *CanvasService) storeError(err error, name string) error {
	switch {
	case errors.Is(err, canvas.ErrInvalidName):
		return Errorf(KindInvalid, "canvas name %q is not allowed: use a short lowercase name like release-notes (letters, digits, dots, hyphens, underscores)", name)
	case errors.Is(err, canvas.ErrInvalidWorkspace):
		return Errorf(KindInvalid, "workspace is not a valid workspace directory name")
	case errors.Is(err, canvas.ErrNotFound):
		return Errorf(KindNotFound, "no canvas named %q in this workspace", name)
	default:
		return Wrap(err, KindInternal, "canvas %q", name)
	}
}

func (s *CanvasService) notify(ctx context.Context, author canvas.Author) {
	s.events.Publish(ctx, events.CanvasUpdated{Session: author.Session, HiveSession: author.HiveSession})
}

// plainTitle decodes HTML entities. A title is plain text on every surface,
// and agents often escape one as if it were markup, so "&amp;" would show
// as written.
func plainTitle(title string) string {
	return html.UnescapeString(title)
}

func validateBlock(b *canvas.Block) error {
	b.ID = strings.TrimSpace(b.ID)
	b.Title = plainTitle(b.Title)
	if b.ID == "" {
		return Errorf(KindInvalid, "a block needs an id")
	}
	if len(b.ID) > maxCanvasBlockIDLength {
		return Errorf(KindInvalid, "block id is too long (%d chars max)", maxCanvasBlockIDLength)
	}
	switch b.Kind {
	case canvas.KindMarkdown:
		if b.Body == "" {
			return Errorf(KindInvalid, "a markdown block needs a body")
		}
		if len(b.Body) > maxCanvasBodyBytes {
			return Errorf(KindInvalid, "block body is too large (%d bytes max)", maxCanvasBodyBytes)
		}
		if b.URL != "" {
			return Errorf(KindInvalid, "a markdown block carries no url; use a link block")
		}
	case canvas.KindHTML:
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
	case canvas.KindLink:
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
		return Errorf(KindInvalid, "unknown block kind %q; use %q, %q or %q", b.Kind, canvas.KindMarkdown, canvas.KindHTML, canvas.KindLink)
	}
	return nil
}

// validateLinkURL mirrors the frontend's link filter: only schemes the pane
// will actually open are accepted, so a block never renders a link the click
// handler then refuses.
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
