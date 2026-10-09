// Package canvas owns the canvas files: one JSON file per canvas at
// <owner dir>/canvases/<name>.json. A canvas is a named artifact an agent
// produced. It belongs to an owner that outlives its author: a chat's canvas
// to the agent workspace, in the workspace folder, and a hive session's to
// the repository, in the repository's hive context directory
// (ADR canvases-are-named-files-in-the-workspace-folder-served-over-their-own-mcp-entry,
// ADR a-code-session-s-canvases-belong-to-its-repository-and-live-in-the-hive-context-directory).
package canvas

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/pkg/atomicfile"
)

// ErrInvalidWorkspace reports an owner that is neither a single local path
// component (a workspace) nor two of them joined by a slash (a repository).
// The store is a leaf, so it re-checks what validWorkspaceDir already
// enforced above it rather than importing anything.
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

// ErrInvalidFrontmatter reports metadata that cannot be represented as safe,
// flat YAML front matter. The two timestamp keys are owned by the canvas.
var ErrInvalidFrontmatter = errors.New("canvas: invalid front matter")

const (
	KindMarkdown = "markdown"
	KindLink     = "link"
	KindHTML     = "html"
)

// DirName is the app-owned directory inside an owner's folder. The workspace
// generator reconciles only its own subtrees and hive's context prune skips
// this name, so nothing else ever writes or prunes here.
const DirName = "canvases"

const (
	maxNameLength          = 100
	maxFrontmatterKeys     = 50
	maxFrontmatterKeyBytes = 64
	maxFrontmatterText     = 4096
	maxFrontmatterList     = 50
	maxFrontmatterBytes    = 64 * 1024
)

// namePattern is the canvas-name slug rule: lowercase alphanumeric with
// dots, hyphens and underscores inside. Lowercase-only because the file name
// is the identity and macOS file systems are case-insensitive — "Report" and
// "report" must not be two canvases that collide on disk. No leading dot, so
// a name can never shadow a generated dot-directory.
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`)

var frontmatterKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

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

// Canvas is one named surface, blocks in display order. Workspace is the
// owner key: a workspace's directory name, or RepositoryOwner's "owner/repo".
// Session and HiveSession are the chat or the hive session that created it —
// provenance for labeling, never authorization: a canvas belongs to its
// owner, not to its author.
type Canvas struct {
	Workspace   string         `json:"workspace"`
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Session     string         `json:"session"`
	HiveSession string         `json:"hiveSession,omitempty"`
	CreatedAt   int64          `json:"createdAt"`
	UpdatedAt   int64          `json:"updatedAt"`
	Frontmatter map[string]any `json:"frontmatter,omitempty"`
	Blocks      []Block        `json:"blocks"`
}

// Author is who wrote a canvas first: a chat by its record id, or a hive
// session by its id. At most one is set; neither is an agent outside both,
// writing to GlobalOwner.
type Author struct {
	Session     string
	HiveSession string
}

// UnmarshalJSON accepts numeric chat ids written before chats moved to UUIDs.
// A later write keeps the original author, so the file keeps the number;
// CanvasService maps it to the chat's UUID on the way out.
func (c *Canvas) UnmarshalJSON(data []byte) error {
	type alias Canvas
	decoded := struct {
		Session json.RawMessage `json:"session"`
		*alias
	}{alias: (*alias)(c)}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if len(decoded.Session) == 0 || string(decoded.Session) == "null" {
		return nil
	}
	if err := json.Unmarshal(decoded.Session, &c.Session); err == nil {
		return nil
	}
	var legacy int64
	if err := json.Unmarshal(decoded.Session, &legacy); err != nil {
		return fmt.Errorf("decode canvas session: %w", err)
	}
	if legacy > 0 {
		c.Session = strconv.FormatInt(legacy, 10)
	}
	return nil
}

// GlobalOwner is the owner key shared by every agent that runs outside a chat
// and outside a hive session. The @ keeps it out of the workspace names hive
// creates.
const GlobalOwner = "@global"

// RepositoryOwner is the owner key of a repository's canvases. A workspace's
// key is one path component, so the slash keeps the two apart.
func RepositoryOwner(owner, repo string) string {
	return owner + "/" + repo
}

// Meta is one row of a workspace listing: everything the pane's picker needs
// without loading block content.
type Meta struct {
	Workspace   string `json:"workspace"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Session     string `json:"session"`
	HiveSession string `json:"hiveSession,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
	BlockCount  int    `json:"blockCount"`
}

// Markdown renders a canvas as one standalone document. Automatic timestamps
// and agent-authored metadata form a YAML header; the body contains the canvas
// title and blocks. An html block is emitted as sanitized markup. It is the
// export shape behind the pane's copy and save actions, so both always agree.
func Markdown(c Canvas) (string, error) {
	header, err := markdownFrontmatter(c)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(header)
	b.WriteString("---\n\n")
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
	return b.String(), nil
}

func markdownFrontmatter(c Canvas) ([]byte, error) {
	if err := ValidateFrontmatter(c.Frontmatter); err != nil {
		return nil, err
	}
	root := yaml.Node{Kind: yaml.MappingNode}
	appendYAMLPair := func(key string, value any) error {
		keyNode := yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
		var valueNode yaml.Node
		if err := valueNode.Encode(value); err != nil {
			return fmt.Errorf("encode front matter %q: %w", key, err)
		}
		root.Content = append(root.Content, &keyNode, &valueNode)
		return nil
	}
	if err := appendYAMLPair("created_at", time.UnixMilli(c.CreatedAt).UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	if err := appendYAMLPair("updated_at", time.UnixMilli(c.UpdatedAt).UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(c.Frontmatter))
	for key := range c.Frontmatter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := appendYAMLPair(key, c.Frontmatter[key]); err != nil {
			return nil, err
		}
	}
	return yaml.Marshal(&root)
}

// Roots locates canvases on disk by what owns them.
type Roots struct {
	// Workspaces is the agent-workspace root. A workspace's canvases sit in
	// its folder there.
	Workspaces string
	// Repositories answers hive's context root, under which a repository's
	// canvases sit at <owner>/<repo>. It is asked per call because hive's
	// config can move the root while the app runs. Nil, or an empty answer,
	// refuses repository owners.
	Repositories func() string
	// Global holds GlobalOwner's canvases directory. Empty refuses the
	// global owner.
	Global string
}

// Store reads and writes canvas files under Roots. The mutex serializes
// read-modify-write cycles; the MCP server and the HTTP reads run in this one
// process, so no cross-process coordination is needed.
type Store struct {
	roots Roots
	mu    sync.Mutex
	now   func() time.Time
}

func NewStore(roots Roots) *Store {
	return &Store{roots: roots, now: time.Now}
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
// — an existing id then moves there, still keeping its CreatedAt. author is
// recorded at creation and never changes; a non-empty title replaces the
// stored one. Returns the canvas after the write.
func (s *Store) Upsert(workspace, name string, author Author, title, before string, blocks ...Block) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.load(workspace, name)
	if err != nil {
		return Canvas{}, err
	}
	nowMillis := s.now().UnixMilli()
	if !ok {
		c = Canvas{
			Workspace: workspace, Name: name, Session: author.Session, HiveSession: author.HiveSession,
			CreatedAt: nowMillis, Frontmatter: map[string]any{}, Blocks: []Block{},
		}
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

// SetFrontmatter replaces every editable metadata field on an existing
// canvas. The canvas-owned created_at and updated_at fields are derived from
// its timestamps and never stored in this map.
func (s *Store) SetFrontmatter(workspace, name string, frontmatter map[string]any) (Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ValidateFrontmatter(frontmatter); err != nil {
		return Canvas{}, err
	}
	c, ok, err := s.load(workspace, name)
	if err != nil {
		return Canvas{}, err
	}
	if !ok {
		return Canvas{}, fmt.Errorf("%w: %s/%s", ErrNotFound, workspace, name)
	}
	c.Frontmatter = cloneFrontmatter(frontmatter)
	c.UpdatedAt = s.now().UnixMilli()
	if err := s.write(c); err != nil {
		return Canvas{}, err
	}
	return c, nil
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

	dir, err := s.dir(workspace)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
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
			Workspace: c.Workspace, Name: c.Name, Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, BlockCount: len(c.Blocks),
		})
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].UpdatedAt > metas[j].UpdatedAt })
	return metas, nil
}

// Delete removes one canvas file and returns what was deleted. The caller can
// use its author to publish the same update signal as other mutations.
func (s *Store) Delete(workspace, name string) (Canvas, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.load(workspace, name)
	if err != nil || !ok {
		return Canvas{}, ok, err
	}
	path, err := s.path(workspace, name)
	if err != nil {
		return Canvas{}, false, err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Canvas{}, false, nil
		}
		return Canvas{}, false, fmt.Errorf("canvas: delete %s/%s: %w", workspace, name, err)
	}
	return c, true, nil
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
	if c.Frontmatter == nil {
		c.Frontmatter = map[string]any{}
	}
	if err := ValidateFrontmatter(c.Frontmatter); err != nil {
		return Canvas{}, false, fmt.Errorf("canvas: parse %s/%s: %w", workspace, name, err)
	}
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
	if err := atomicfile.Write(path, data, 0o600); err != nil {
		return fmt.Errorf("canvas: replace: %w", err)
	}
	return nil
}

func (s *Store) path(workspace, name string) (string, error) {
	dir, err := s.dir(workspace)
	if err != nil {
		return "", err
	}
	if !validName(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return filepath.Join(dir, name+".json"), nil
}

// dir is the canvases directory of one owner. The repository layout is
// hive's own (config.RepoContextDir), restated because this package imports
// nothing; a test in the app package holds the two together.
func (s *Store) dir(owner string) (string, error) {
	if owner == GlobalOwner {
		if s.roots.Global == "" {
			return "", fmt.Errorf("%w: %q", ErrInvalidWorkspace, owner)
		}
		return filepath.Join(s.roots.Global, DirName), nil
	}
	repoOwner, repo, isRepository := strings.Cut(owner, "/")
	if !isRepository {
		if !validWorkspace(owner) {
			return "", fmt.Errorf("%w: %q", ErrInvalidWorkspace, owner)
		}
		return filepath.Join(s.roots.Workspaces, owner, DirName), nil
	}
	root := s.repositoriesRoot()
	if root == "" || !validWorkspace(repoOwner) || !validWorkspace(repo) {
		return "", fmt.Errorf("%w: %q", ErrInvalidWorkspace, owner)
	}
	return filepath.Join(root, repoOwner, repo, DirName), nil
}

func (s *Store) repositoriesRoot() string {
	if s.roots.Repositories == nil {
		return ""
	}
	return s.roots.Repositories()
}

// Repositories returns the owner key of every repository that holds a
// canvas, sorted. A repository whose sessions are all gone still lists: its
// canvases outlive them.
func (s *Store) Repositories() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	root := s.repositoriesRoot()
	if root == "" {
		return []string{}, nil
	}
	owners, err := subdirectories(root)
	if err != nil {
		return nil, fmt.Errorf("canvas: list repositories: %w", err)
	}
	keys := []string{}
	for _, owner := range owners {
		repos, err := subdirectories(filepath.Join(root, owner))
		if err != nil {
			return nil, fmt.Errorf("canvas: list repositories: %w", err)
		}
		for _, repo := range repos {
			entries, err := os.ReadDir(filepath.Join(root, owner, repo, DirName))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("canvas: list repositories: %w", err)
			}
			holdsCanvas := slices.ContainsFunc(entries, func(entry os.DirEntry) bool {
				_, ok := nameFromFilename(entry.Name())
				return ok
			})
			if holdsCanvas {
				keys = append(keys, RepositoryOwner(owner, repo))
			}
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// subdirectories names the directories inside dir, following a link to one.
// A dir that does not exist has none.
func subdirectories(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if info, err := os.Stat(filepath.Join(dir, entry.Name())); err == nil && info.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// validWorkspace is the one-path-component rule validWorkspaceDir enforces
// above this package. A repository key is held to it one component at a time.
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

// ValidateFrontmatter accepts a flat YAML-style mapping whose values are
// scalars or scalar lists. Nested objects and lists are refused because the
// reader presents this as a compact property list, not a second document.
func ValidateFrontmatter(frontmatter map[string]any) error {
	if len(frontmatter) > maxFrontmatterKeys {
		return fmt.Errorf("%w: too many fields (%d max)", ErrInvalidFrontmatter, maxFrontmatterKeys)
	}
	for key, value := range frontmatter {
		if len(key) > maxFrontmatterKeyBytes || !frontmatterKeyPattern.MatchString(key) {
			return fmt.Errorf("%w: key %q must use letters, digits, dots, hyphens, or underscores", ErrInvalidFrontmatter, key)
		}
		if strings.EqualFold(key, "created_at") || strings.EqualFold(key, "updated_at") {
			return fmt.Errorf("%w: %q is managed by Hive", ErrInvalidFrontmatter, key)
		}
		if err := validateFrontmatterValue(value, false); err != nil {
			return fmt.Errorf("%w: %q: %w", ErrInvalidFrontmatter, key, err)
		}
	}
	data, err := json.Marshal(frontmatter)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFrontmatter, err)
	}
	if len(data) > maxFrontmatterBytes {
		return fmt.Errorf("%w: metadata is too large (%d bytes max)", ErrInvalidFrontmatter, maxFrontmatterBytes)
	}
	return nil
}

func validateFrontmatterValue(value any, inList bool) error {
	switch value := value.(type) {
	case nil, bool, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("number must be finite")
		}
		return nil
	case string:
		if len(value) > maxFrontmatterText {
			return fmt.Errorf("text is too long (%d bytes max)", maxFrontmatterText)
		}
		return nil
	case []any:
		if inList {
			return errors.New("nested lists are not allowed")
		}
		if len(value) > maxFrontmatterList {
			return fmt.Errorf("list has too many values (%d max)", maxFrontmatterList)
		}
		for _, item := range value {
			if err := validateFrontmatterValue(item, true); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("value has unsupported type %T; use a scalar or a list of scalars", value)
	}
}

func cloneFrontmatter(frontmatter map[string]any) map[string]any {
	cloned := make(map[string]any, len(frontmatter))
	for key, value := range frontmatter {
		if list, ok := value.([]any); ok {
			cloned[key] = append([]any(nil), list...)
			continue
		}
		cloned[key] = value
	}
	return cloned
}
