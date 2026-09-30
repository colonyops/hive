// Package canvas owns the canvas files inside a workspace folder: one JSON
// file per canvas at <root>/<workspace>/canvases/<name>.json, where root is
// the agent-workspace root. A canvas is a named artifact a chat produced —
// it lives beside the workspace's authored files, is browsable in place, and
// outlives the chat that made it
// (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry).
package canvas

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrInvalidWorkspace reports a workspace value that is not a single local
// path component. The store is a leaf, so it re-checks what
// validWorkspaceDir already enforced above it rather than importing anything.
var ErrInvalidWorkspace = errors.New("canvas: invalid workspace name")

// ErrInvalidName reports a canvas name outside the slug rule below.
var ErrInvalidName = errors.New("canvas: invalid canvas name")

// ErrNotFound reports an operation on a canvas that does not exist. Distinct
// from an invalid name: the name is fine, there is just no file behind it.
var ErrNotFound = errors.New("canvas: not found")

// ErrAnchorNotFound reports a before-anchor id that names no block on the
// canvas — including a block the same write is moving, which cannot anchor
// itself.
var ErrAnchorNotFound = errors.New("canvas: anchor block not found")

const (
	KindMarkdown = "markdown"
	KindLink     = "link"
	KindHTML     = "html"
)

// canvasesDirName is the app-owned directory inside a workspace folder.
// The workspace generator reconciles only its own subtrees, so nothing else
// ever writes or prunes here.
const canvasesDirName = "canvases"

const maxNameLength = 100

// namePattern is the canvas-name slug rule: lowercase alphanumeric with
// dots, hyphens and underscores inside. Lowercase-only because the file name
// is the identity and macOS file systems are case-insensitive — "Report" and
// "report" must not be two canvases that collide on disk. No leading dot, so
// a name can never shadow a generated dot-directory.
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`)

// Block is one entry on a canvas. Kind decides which content field is set:
// markdown and html carry Body, link carries URL. An html block's Body is
// the agent's source verbatim — SanitizeHTML runs on the way out, never on
// the way in, so read_canvas shows the agent what it wrote. Timestamps are
// unix milliseconds, matching AgentWorkspaceSession.
type Block struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title,omitempty"`
	Body      string `json:"body,omitempty"`
	URL       string `json:"url,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Canvas is one named surface, blocks in display order. Session is the chat
// that created it — provenance for labeling, never authorization: a canvas
// belongs to its workspace, not to the chat.
type Canvas struct {
	Workspace string  `json:"workspace"`
	Name      string  `json:"name"`
	Title     string  `json:"title,omitempty"`
	Session   int64   `json:"session"`
	CreatedAt int64   `json:"createdAt"`
	UpdatedAt int64   `json:"updatedAt"`
	Blocks    []Block `json:"blocks"`
}

// Meta is one row of a workspace listing: everything the pane's picker needs
// without loading block content.
type Meta struct {
	Workspace  string `json:"workspace"`
	Name       string `json:"name"`
	Title      string `json:"title,omitempty"`
	Session    int64  `json:"session"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	BlockCount int    `json:"blockCount"`
}

// Markdown renders a canvas as one standalone document: the canvas title as
// a top-level heading, each markdown block's title demoted beneath it, and
// link blocks as plain markdown links. An html block is emitted as its
// sanitized markup, which most markdown viewers render; the hv- class names
// mean nothing outside the app, so an export keeps the structure and loses
// the styling. It is the export shape behind the pane's copy and save
// actions, so both always agree.
func Markdown(c Canvas) string {
	var b strings.Builder
	if c.Title != "" {
		b.WriteString("# " + c.Title + "\n\n")
	}
	for i, block := range c.Blocks {
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch block.Kind {
		case KindLink:
			b.WriteString("[" + block.Title + "](" + block.URL + ")")
		case KindHTML:
			if block.Title != "" {
				b.WriteString("## " + block.Title + "\n\n")
			}
			b.WriteString(SanitizeHTML(block.Body))
		default:
			if block.Title != "" {
				b.WriteString("## " + block.Title + "\n\n")
			}
			b.WriteString(block.Body)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// Store reads and writes canvas files under the agent-workspace root. The
// mutex serializes read-modify-write cycles; the MCP server and the HTTP
// reads run in this one process, so no cross-process coordination is needed.
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func NewStore(root string) *Store {
	return &Store{root: root, now: time.Now}
}

// Load returns one canvas, reporting false without error when none has ever
// been written. A file that exists but cannot be parsed is an error, never a
// silently blank canvas.
func (s *Store) Load(workspace, name string) (Canvas, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(workspace, name)
}

// Upsert writes blocks in order in one atomic file write, creating the
// canvas on the first: an id already on the canvas is replaced in place,
// keeping its position and CreatedAt; a new id appends. A non-empty before
// names an existing block id every written block is instead placed ahead of
// — an existing id then moves there, still keeping its CreatedAt. session is
// recorded at creation and never changes; a non-empty title replaces the
// stored one. Returns the canvas after the write.
func (s *Store) Upsert(workspace, name string, session int64, title, before string, blocks ...Block) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.load(workspace, name)
	if err != nil {
		return Canvas{}, err
	}
	nowMillis := s.now().UnixMilli()
	if !ok {
		c = Canvas{Workspace: workspace, Name: name, Session: session, CreatedAt: nowMillis, Blocks: []Block{}}
	}
	if title != "" {
		c.Title = title
	}

	for _, b := range blocks {
		b.CreatedAt = nowMillis
		b.UpdatedAt = nowMillis
		if i := blockIndex(c.Blocks, b.ID); i >= 0 {
			b.CreatedAt = c.Blocks[i].CreatedAt
			if before == "" {
				c.Blocks[i] = b
				continue
			}
			c.Blocks = slices.Delete(c.Blocks, i, i+1)
		}
		if before == "" {
			c.Blocks = append(c.Blocks, b)
			continue
		}
		anchor := blockIndex(c.Blocks, before)
		if anchor < 0 {
			return Canvas{}, fmt.Errorf("%w: %q", ErrAnchorNotFound, before)
		}
		c.Blocks = slices.Insert(c.Blocks, anchor, b)
	}
	c.UpdatedAt = nowMillis

	if err := s.write(c); err != nil {
		return Canvas{}, err
	}
	return c, nil
}

func blockIndex(blocks []Block, id string) int {
	for i, b := range blocks {
		if b.ID == id {
			return i
		}
	}
	return -1
}

// Remove deletes one block by id, reporting whether it was present. A canvas
// that does not exist is ErrNotFound.
func (s *Store) Remove(workspace, name, blockID string) (Canvas, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.load(workspace, name)
	if err != nil {
		return Canvas{}, false, err
	}
	if !ok {
		return Canvas{}, false, fmt.Errorf("%w: %s/%s", ErrNotFound, workspace, name)
	}
	kept := c.Blocks[:0]
	removed := false
	for _, b := range c.Blocks {
		if b.ID == blockID {
			removed = true
			continue
		}
		kept = append(kept, b)
	}
	if !removed {
		return c, false, nil
	}
	c.Blocks = kept
	c.UpdatedAt = s.now().UnixMilli()
	if err := s.write(c); err != nil {
		return Canvas{}, false, err
	}
	return c, true, nil
}

// Clear empties an existing canvas but keeps it: the file, its title and its
// CreatedAt survive, so a clear reads as a fresh layout in the same pane,
// not a deletion. A canvas that does not exist is ErrNotFound.
func (s *Store) Clear(workspace, name string) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.load(workspace, name)
	if err != nil {
		return Canvas{}, err
	}
	if !ok {
		return Canvas{}, fmt.Errorf("%w: %s/%s", ErrNotFound, workspace, name)
	}
	c.Blocks = []Block{}
	c.UpdatedAt = s.now().UnixMilli()
	if err := s.write(c); err != nil {
		return Canvas{}, err
	}
	return c, nil
}

// List returns a workspace's canvases, most recently updated first. A
// workspace with none — including one whose canvases directory does not
// exist — answers empty.
func (s *Store) List(workspace string) ([]Meta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !validWorkspace(workspace) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidWorkspace, workspace)
	}
	entries, err := os.ReadDir(filepath.Join(s.root, workspace, canvasesDirName))
	if errors.Is(err, os.ErrNotExist) {
		return []Meta{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("canvas: list %s: %w", workspace, err)
	}

	metas := make([]Meta, 0, len(entries))
	for _, entry := range entries {
		name, ok := nameFromFilename(entry.Name())
		if !ok {
			continue
		}
		c, found, err := s.load(workspace, name)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		metas = append(metas, Meta{
			Workspace: c.Workspace, Name: c.Name, Title: c.Title, Session: c.Session,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, BlockCount: len(c.Blocks),
		})
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].UpdatedAt > metas[j].UpdatedAt })
	return metas, nil
}

// Delete removes one canvas file, reporting whether it existed.
func (s *Store) Delete(workspace, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.path(workspace, name)
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("canvas: delete %s/%s: %w", workspace, name, err)
	}
	return true, nil
}

func (s *Store) load(workspace, name string) (Canvas, bool, error) {
	path, err := s.path(workspace, name)
	if err != nil {
		return Canvas{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Canvas{}, false, nil
	}
	if err != nil {
		return Canvas{}, false, fmt.Errorf("canvas: read %s/%s: %w", workspace, name, err)
	}
	var c Canvas
	if err := json.Unmarshal(data, &c); err != nil {
		return Canvas{}, false, fmt.Errorf("canvas: parse %s/%s: %w", workspace, name, err)
	}
	c.Workspace = workspace
	c.Name = name
	if c.Blocks == nil {
		c.Blocks = []Block{}
	}
	return c, true, nil
}

// write replaces the file atomically: a temp file in the same directory, then
// a rename, so a crash mid-write cannot truncate a canvas.
func (s *Store) write(c Canvas) error {
	path, err := s.path(c.Workspace, c.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("canvas: create dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("canvas: encode %s/%s: %w", c.Workspace, c.Name, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("canvas: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("canvas: replace: %w", err)
	}
	return nil
}

func (s *Store) path(workspace, name string) (string, error) {
	if !validWorkspace(workspace) {
		return "", fmt.Errorf("%w: %q", ErrInvalidWorkspace, workspace)
	}
	if !validName(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return filepath.Join(s.root, workspace, canvasesDirName, name+".json"), nil
}

// validWorkspace is the one-path-component rule validWorkspaceDir enforces
// above this package.
func validWorkspace(dir string) bool {
	if dir == "" || dir == "." {
		return false
	}
	return filepath.Base(dir) == dir && filepath.IsLocal(dir)
}

// validName reports whether name is a canvas name the store will accept.
func validName(name string) bool {
	return len(name) <= maxNameLength && namePattern.MatchString(name)
}

func nameFromFilename(filename string) (string, bool) {
	base, ok := strings.CutSuffix(filename, ".json")
	if !ok || !validName(base) {
		return "", false
	}
	return base, true
}
