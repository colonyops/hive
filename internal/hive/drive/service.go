// Package drive is the Hive Drive service: the document tree under the
// context root, served from one in-memory snapshot per space. A space is
// scanned the first time something reads it, and every write through the
// service keeps its snapshot current and publishes drive.changed.
package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/atomicfile"
	"github.com/colonyops/hive/pkg/fsx"
	"github.com/colonyops/hive/pkg/logutils"
)

const (
	// frontmatterHeadBytes bounds what a scan reads from a markdown file:
	// front matter past 8 KiB is not indexed, and the file is not an error.
	frontmatterHeadBytes = 8 * 1024
	defaultSearchLimit   = 50
	dirPerm              = 0o755
	filePerm             = 0o644
)

// Hit is one search result.
type Hit struct {
	Space drive.Space `json:"space"`
	Entry drive.Entry `json:"entry"`
}

type spaceSnapshot struct {
	root drive.Entry
}

// Service serves the drive. Reads of a scanned space never touch the disk;
// a mutation does its file operation, files the result in the snapshot, and
// publishes once.
type Service struct {
	logger zerolog.Logger
	bus    *events.EventBus
	now    func() time.Time

	mu     sync.RWMutex
	cfg    *config.Config
	git    git.Git
	spaces map[string]*spaceSnapshot
}

func NewService(logger zerolog.Logger, cfg *config.Config, gitClient git.Git, bus *events.EventBus) *Service {
	return &Service{
		logger: logutils.Component(logger, "drive"),
		bus:    bus,
		now:    time.Now,
		cfg:    cfg,
		git:    gitClient,
		spaces: map[string]*spaceSnapshot{},
	}
}

// Reconfigure is what Engine.Reload calls. The snapshots describe one root,
// so a changed ContextDir drops them; anything else only swaps the config
// and the git executor built for it.
func (s *Service) Reconfigure(cfg *config.Config, gitClient git.Git) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.ContextDir() != s.cfg.ContextDir() {
		s.spaces = map[string]*spaceSnapshot{}
	}
	s.cfg = cfg
	s.git = gitClient
}

// Root is the drive root: the context directory.
func (s *Service) Root() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.ContextDir()
}

// Spaces lists every space the root holds: a repository space per
// <owner>/<repo> directory pair, a workspace space per directory under
// workspaces/, and the shared space, which is listed whether or not its
// directory exists. Repositories come first, sorted, then workspaces, then
// shared.
func (s *Service) Spaces(context.Context) ([]drive.Space, error) {
	root := s.Root()
	owners, err := subdirectories(root)
	if err != nil {
		return nil, fmt.Errorf("drive: list spaces: %w", err)
	}
	var repositories, workspaces []drive.Space
	for _, owner := range owners {
		switch {
		case owner == drive.WorkspacesDir:
			names, err := subdirectories(filepath.Join(root, owner))
			if err != nil {
				return nil, fmt.Errorf("drive: list spaces: %w", err)
			}
			for _, name := range names {
				space, err := drive.WorkspaceSpace(name)
				if err != nil {
					s.logger.Debug().Err(err).Str("dir", name).Msg("skipping workspace directory")
					continue
				}
				workspaces = append(workspaces, space)
			}
		case strings.EqualFold(owner, drive.SharedKey) || strings.EqualFold(owner, drive.WorkspacesDir):
			continue
		default:
			repos, err := subdirectories(filepath.Join(root, owner))
			if err != nil {
				return nil, fmt.Errorf("drive: list spaces: %w", err)
			}
			for _, repo := range repos {
				space, err := drive.RepositorySpace(owner, repo)
				if err != nil {
					s.logger.Debug().Err(err).Str("dir", owner+"/"+repo).Msg("skipping repository directory")
					continue
				}
				repositories = append(repositories, space)
			}
		}
	}
	spaces := make([]drive.Space, 0, len(repositories)+len(workspaces)+1)
	spaces = append(spaces, repositories...)
	spaces = append(spaces, workspaces...)
	return append(spaces, drive.Shared), nil
}

// subdirectories names the directories inside dir, following a link to one
// and skipping dot entries. A dir that does not exist has none.
func subdirectories(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if info, err := os.Stat(filepath.Join(dir, entry.Name())); err == nil && info.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// SpaceForRemote is the repository space a git remote URL belongs to.
func (s *Service) SpaceForRemote(remote string) (drive.Space, bool) {
	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return drive.Space{}, false
	}
	space, err := drive.RepositorySpace(owner, repo)
	return space, err == nil
}

// SpaceDir is the directory of a space: Root()/Key. The key is re-parsed so
// a key that is not one of the three forms, including one that would climb
// out of the root, is ErrInvalidSpace.
func (s *Service) SpaceDir(space drive.Space) (string, error) {
	return spaceDir(s.Root(), space)
}

func spaceDir(root string, space drive.Space) (string, error) {
	parsed, err := drive.ParseSpaceKey(space.Key)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(parsed.Key)), nil
}

// ResolveSpace is the space a CLI invocation means: --repo owner/repo,
// --shared, or the repository behind the current directory's origin remote.
func (s *Service) ResolveSpace(ctx context.Context, repo string, shared bool) (drive.Space, error) {
	if shared {
		return drive.Shared, nil
	}
	if repo != "" {
		owner, name, ok := strings.Cut(repo, "/")
		if !ok {
			return drive.Space{}, fmt.Errorf("invalid repo format, expected owner/repo: %s", repo)
		}
		return drive.RepositorySpace(owner, name)
	}

	s.mu.RLock()
	gitClient := s.git
	s.mu.RUnlock()
	remote, err := gitClient.RemoteURL(ctx, ".")
	if err != nil {
		return drive.Space{}, fmt.Errorf("detect remote (are you in a git repository?): %w", err)
	}
	space, ok := s.SpaceForRemote(remote)
	if !ok {
		return drive.Space{}, fmt.Errorf("could not extract owner/repo from remote: %s", remote)
	}
	return space, nil
}

// ResolveDir is SpaceDir(ResolveSpace(...)).
func (s *Service) ResolveDir(ctx context.Context, repo string, shared bool) (string, error) {
	space, err := s.ResolveSpace(ctx, repo, shared)
	if err != nil {
		return "", err
	}
	return s.SpaceDir(space)
}

// Tree is the space's whole tree, scanned on first use. The result is a deep
// copy: a caller may keep or change it without touching the snapshot. A space
// whose directory does not exist is an empty root, not an error.
func (s *Service) Tree(_ context.Context, space drive.Space) (drive.Entry, error) {
	s.mu.RLock()
	if snap, ok := s.spaces[space.Key]; ok {
		root := cloneEntry(snap.root)
		s.mu.RUnlock()
		return root, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.snapshot(space)
	if err != nil {
		return drive.Entry{}, err
	}
	return cloneEntry(snap.root), nil
}

// snapshot answers the space's snapshot, scanning on first use. The caller
// holds s.mu for writing.
func (s *Service) snapshot(space drive.Space) (*spaceSnapshot, error) {
	if snap, ok := s.spaces[space.Key]; ok {
		return snap, nil
	}
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return nil, err
	}
	root, err := s.scan(dir)
	if err != nil {
		return nil, fmt.Errorf("drive: scan %s: %w", space.Key, err)
	}
	snap := &spaceSnapshot{root: root}
	s.spaces[space.Key] = snap
	return snap, nil
}

// Search matches query as a case-insensitive substring of an entry's path or
// title, in every space, scanning the ones no one has read yet. Hits come
// grouped in Spaces order and sorted by path inside a space, without
// children; limit caps them, and 0 means defaultSearchLimit. An empty query
// matches nothing.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	spaces, err := s.Spaces(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	hits := []Hit{}
	for _, space := range spaces {
		snap, err := s.snapshot(space)
		if err != nil {
			return nil, err
		}
		var found []Hit
		for _, entry := range flatten(snap.root) {
			if strings.Contains(strings.ToLower(entry.Path), needle) || strings.Contains(strings.ToLower(entry.Title), needle) {
				entry.Children = nil
				found = append(found, Hit{Space: space, Entry: cloneEntry(entry)})
			}
		}
		slices.SortFunc(found, func(a, b Hit) int { return strings.Compare(a.Entry.Path, b.Entry.Path) })
		hits = append(hits, found...)
		if len(hits) >= limit {
			break
		}
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// Read is the entry at path, built from the file as it is now, and the file's
// bytes. A folder is not a document and is ErrInvalidPath.
func (s *Service) Read(_ context.Context, space drive.Space, path string) (drive.Entry, []byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	full, err := s.documentFile(space, path)
	if err != nil {
		return drive.Entry{}, nil, err
	}
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return drive.Entry{}, nil, notFound(space, path)
	}
	if err != nil {
		return drive.Entry{}, nil, fmt.Errorf("drive: read %s/%s: %w", space.Key, path, err)
	}
	if info.IsDir() {
		return drive.Entry{}, nil, fmt.Errorf("%w: %q is a folder", drive.ErrInvalidPath, path)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return drive.Entry{}, nil, fmt.Errorf("drive: read %s/%s: %w", space.Key, path, err)
	}
	return s.fileEntry(full, path, info), data, nil
}

// Write replaces a markdown or html document with body, creating the folders
// above it. Anything else is ErrNotWritable: a canvas changes through the
// canvas operations, and the drive writes no other kind.
func (s *Service) Write(_ context.Context, space drive.Space, path string, body []byte) (drive.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	full, err := s.documentFile(space, path)
	if err != nil {
		return drive.Entry{}, err
	}
	if kind := drive.KindOf(path); kind != drive.KindMarkdown && kind != drive.KindHtml {
		return drive.Entry{}, fmt.Errorf("%w: %q", drive.ErrNotWritable, path)
	}
	if info, err := os.Lstat(full); err == nil && info.IsDir() {
		return drive.Entry{}, fmt.Errorf("%w: %q is a folder", drive.ErrExists, path)
	}
	if err := os.MkdirAll(filepath.Dir(full), dirPerm); err != nil {
		return drive.Entry{}, fmt.Errorf("drive: create folder for %s/%s: %w", space.Key, path, err)
	}
	if err := atomicfile.Write(full, body, filePerm); err != nil {
		return drive.Entry{}, fmt.Errorf("drive: write %s/%s: %w", space.Key, path, err)
	}
	entry, err := s.refresh(space, path)
	if err != nil {
		return drive.Entry{}, err
	}
	s.publish(space, path)
	return entry, nil
}

// CreateFolder makes one folder, and the folders above it. A path that
// exists, as anything, is ErrExists.
func (s *Service) CreateFolder(_ context.Context, space drive.Space, path string) (drive.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	full, err := s.documentFile(space, path)
	if err != nil {
		return drive.Entry{}, err
	}
	if _, err := os.Lstat(full); err == nil {
		return drive.Entry{}, fmt.Errorf("%w: %q", drive.ErrExists, path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return drive.Entry{}, fmt.Errorf("drive: create folder %s/%s: %w", space.Key, path, err)
	}
	if err := os.MkdirAll(full, dirPerm); err != nil {
		return drive.Entry{}, fmt.Errorf("drive: create folder %s/%s: %w", space.Key, path, err)
	}
	entry, err := s.refresh(space, path)
	if err != nil {
		return drive.Entry{}, err
	}
	s.publish(space, path)
	return entry, nil
}

// Move renames a document or a folder inside its space, creating the folders
// above the target. The target must not exist (ErrExists), and a folder
// cannot move into itself (ErrInvalidPath). A moved folder's descendants are
// re-keyed in the snapshot rather than rescanned.
func (s *Service) Move(_ context.Context, space drive.Space, from, to string) (drive.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	src, err := s.documentFile(space, from)
	if err != nil {
		return drive.Entry{}, err
	}
	dst, err := s.documentFile(space, to)
	if err != nil {
		return drive.Entry{}, err
	}
	if to == from || strings.HasPrefix(to, from+"/") {
		return drive.Entry{}, fmt.Errorf("%w: cannot move %q into itself", drive.ErrInvalidPath, from)
	}
	if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
		return drive.Entry{}, notFound(space, from)
	} else if err != nil {
		return drive.Entry{}, fmt.Errorf("drive: move %s/%s: %w", space.Key, from, err)
	}
	if _, err := os.Lstat(dst); err == nil {
		return drive.Entry{}, fmt.Errorf("%w: %q", drive.ErrExists, to)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return drive.Entry{}, fmt.Errorf("drive: move %s/%s: %w", space.Key, to, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return drive.Entry{}, fmt.Errorf("drive: create folder for %s/%s: %w", space.Key, to, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return drive.Entry{}, fmt.Errorf("drive: move %s/%s to %s: %w", space.Key, from, to, err)
	}

	var entry drive.Entry
	if snap, ok := s.spaces[space.Key]; ok {
		if removed, ok := removeNode(&snap.root, from); ok && removed.Kind == drive.KindDir {
			rekey(&removed, to)
			s.ensureAncestors(&snap.root, space, to)
			upsertNode(&snap.root, removed)
			entry = cloneEntry(removed)
		}
	}
	if entry.Path == "" {
		if entry, err = s.refresh(space, to); err != nil {
			return drive.Entry{}, err
		}
	}
	s.publish(space, from, to)
	return entry, nil
}

// Delete removes a document or an empty folder. A folder with anything in
// it, hidden files included, is ErrNotEmpty.
func (s *Service) Delete(_ context.Context, space drive.Space, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	full, err := s.documentFile(space, path)
	if err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return notFound(space, path)
	}
	if err != nil {
		return fmt.Errorf("drive: delete %s/%s: %w", space.Key, path, err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(full)
		if err != nil {
			return fmt.Errorf("drive: delete %s/%s: %w", space.Key, path, err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("%w: %q", drive.ErrNotEmpty, path)
		}
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("drive: delete %s/%s: %w", space.Key, path, err)
	}
	if snap, ok := s.spaces[space.Key]; ok {
		removeNode(&snap.root, path)
	}
	s.publish(space, path)
	return nil
}

// LoadCanvas is one canvas, reporting false without error when no file
// exists at path. A file that cannot be parsed is an error, never a blank
// canvas.
func (s *Service) LoadCanvas(_ context.Context, space drive.Space, path string) (drive.Canvas, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadCanvas(space, path)
}

// UpsertBlocks writes blocks in order in one file write, creating the canvas
// on the first: an id already on the canvas is replaced in place, keeping its
// position and CreatedAt; a new id appends. A non-empty before names an
// existing block id every written block is placed ahead of instead; an
// existing id then moves there, still keeping its CreatedAt. author is
// recorded at creation and never changes; a non-empty title replaces the
// stored one.
func (s *Service) UpsertBlocks(_ context.Context, space drive.Space, path string, author drive.Author, title, before string, blocks ...drive.Block) (drive.Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.loadCanvas(space, path)
	if err != nil {
		return drive.Canvas{}, err
	}
	nowMillis := s.now().UnixMilli()
	if !ok {
		c = drive.Canvas{
			Space: space.Key, Path: path, Session: author.Session, HiveSession: author.HiveSession,
			CreatedAt: nowMillis, Frontmatter: drive.Frontmatter{}, Blocks: []drive.Block{},
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
			return drive.Canvas{}, fmt.Errorf("%w: %q", drive.ErrAnchorNotFound, before)
		}
		c.Blocks = slices.Insert(c.Blocks, anchor, b)
	}
	c.UpdatedAt = nowMillis

	if err := s.writeCanvas(space, c); err != nil {
		return drive.Canvas{}, err
	}
	return c, nil
}

func blockIndex(blocks []drive.Block, id string) int {
	return slices.IndexFunc(blocks, func(b drive.Block) bool { return b.ID == id })
}

// RemoveBlock deletes one block by id, reporting whether it was present. A
// canvas that does not exist is ErrNotFound.
func (s *Service) RemoveBlock(_ context.Context, space drive.Space, path, blockID string) (drive.Canvas, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.loadCanvas(space, path)
	if err != nil {
		return drive.Canvas{}, false, err
	}
	if !ok {
		return drive.Canvas{}, false, notFound(space, path)
	}
	i := blockIndex(c.Blocks, blockID)
	if i < 0 {
		return c, false, nil
	}
	c.Blocks = slices.Delete(c.Blocks, i, i+1)
	c.UpdatedAt = s.now().UnixMilli()
	if err := s.writeCanvas(space, c); err != nil {
		return drive.Canvas{}, false, err
	}
	return c, true, nil
}

// SetCanvasFrontmatter replaces every editable metadata field on an existing
// canvas. created_at and updated_at derive from the canvas timestamps and are
// never stored in this map.
func (s *Service) SetCanvasFrontmatter(_ context.Context, space drive.Space, path string, fm drive.Frontmatter) (drive.Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := drive.ValidateFrontmatter(fm); err != nil {
		return drive.Canvas{}, err
	}
	c, ok, err := s.loadCanvas(space, path)
	if err != nil {
		return drive.Canvas{}, err
	}
	if !ok {
		return drive.Canvas{}, notFound(space, path)
	}
	c.Frontmatter = cloneFrontmatter(fm)
	if c.Frontmatter == nil {
		c.Frontmatter = drive.Frontmatter{}
	}
	c.UpdatedAt = s.now().UnixMilli()
	if err := s.writeCanvas(space, c); err != nil {
		return drive.Canvas{}, err
	}
	return c, nil
}

// ClearCanvas empties an existing canvas but keeps it: the file, its title
// and its CreatedAt survive, so a clear reads as a fresh layout in the same
// pane, not a deletion.
func (s *Service) ClearCanvas(_ context.Context, space drive.Space, path string) (drive.Canvas, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok, err := s.loadCanvas(space, path)
	if err != nil {
		return drive.Canvas{}, err
	}
	if !ok {
		return drive.Canvas{}, notFound(space, path)
	}
	c.Blocks = []drive.Block{}
	c.UpdatedAt = s.now().UnixMilli()
	if err := s.writeCanvas(space, c); err != nil {
		return drive.Canvas{}, err
	}
	return c, nil
}

// ListCanvases is every canvas in the space's snapshot, most recently
// updated first. A canvas file that cannot be parsed fails the listing.
func (s *Service) ListCanvases(_ context.Context, space drive.Space) ([]drive.CanvasMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap, err := s.snapshot(space)
	if err != nil {
		return nil, err
	}
	metas := []drive.CanvasMeta{}
	for _, entry := range flatten(snap.root) {
		if entry.Kind != drive.KindCanvas {
			continue
		}
		c, ok, err := s.loadCanvas(space, entry.Path)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		metas = append(metas, drive.CanvasMeta{
			Space: c.Space, Path: c.Path, Title: c.Title, Session: c.Session, HiveSession: c.HiveSession,
			CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, BlockCount: len(c.Blocks),
		})
	}
	sort.SliceStable(metas, func(i, j int) bool { return metas[i].UpdatedAt > metas[j].UpdatedAt })
	return metas, nil
}

// ImportLegacyCanvases moves the canvases in from, a canvases directory
// outside the drive, to <space>/canvases/<name>.canvas.json, rewriting each
// file's space and path on the way. A .json that is not a canvas stays; a
// target that exists is skipped with a warning; an emptied from is removed.
// A from that does not exist is 0, nil. The desktop runs this at start for
// the directories only it knows, so it is idempotent.
func (s *Service) ImportLegacyCanvases(_ context.Context, from string, space drive.Space) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return 0, err
	}
	target := filepath.Join(dir, drive.CanvasesDir)
	if filepath.Clean(from) == target {
		return 0, nil
	}
	entries, err := os.ReadDir(from)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("drive: import canvases from %s: %w", from, err)
	}

	var moved []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		src := filepath.Join(from, name)
		data, err := os.ReadFile(src)
		if err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas not readable, left in place")
			continue
		}
		if !isCanvasJSON(data) {
			continue
		}
		var c drive.Canvas
		if err := json.Unmarshal(data, &c); err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas does not parse, left in place")
			continue
		}
		path := drive.CanvasesDir + "/" + canvasStem(name) + drive.CanvasExt
		dst := filepath.Join(dir, filepath.FromSlash(path))
		if _, err := os.Lstat(dst); err == nil {
			s.logger.Warn().Str("file", src).Str("target", dst).Msg("legacy canvas target exists, left in place")
			continue
		}
		c.Space = space.Key
		c.Path = path
		out, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return len(moved), fmt.Errorf("drive: encode %s: %w", src, err)
		}
		if err := os.MkdirAll(target, dirPerm); err != nil {
			return len(moved), fmt.Errorf("drive: create %s: %w", target, err)
		}
		if err := atomicfile.Write(dst, out, filePerm); err != nil {
			return len(moved), fmt.Errorf("drive: write %s: %w", dst, err)
		}
		if err := os.Remove(src); err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas copied but not removed")
		}
		moved = append(moved, path)
	}

	if rest, err := os.ReadDir(from); err == nil && len(rest) == 0 {
		if err := os.Remove(from); err != nil {
			s.logger.Warn().Err(err).Str("dir", from).Msg("emptied legacy canvases directory not removed")
		}
	}
	if len(moved) > 0 {
		delete(s.spaces, space.Key)
		s.publish(space, moved...)
	}
	return len(moved), nil
}

// Init creates the context directory and its standard subdirectories,
// answering the ones it made.
func (s *Service) Init(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("create context directory: %w", err)
	}
	var created []string
	for _, subdir := range []string{"research", "plans", "references"} {
		path := filepath.Join(dir, subdir)
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(path, dirPerm); err != nil {
				return nil, fmt.Errorf("create subdirectory %s: %w", subdir, err)
			}
			created = append(created, subdir)
		}
	}
	if len(created) > 0 {
		s.forget(dir)
	}
	return created, nil
}

// CreateSymlink links the current directory's symlink name to dir. True
// means it already existed and pointed there.
func (s *Service) CreateSymlink(dir string) (bool, error) {
	s.mu.RLock()
	symlinkName := s.cfg.Context.SymlinkName
	s.mu.RUnlock()
	symlinkPath := filepath.Join(".", symlinkName)

	if info, err := os.Lstat(symlinkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(symlinkPath)
			if target == dir {
				return true, nil
			}
			return false, fmt.Errorf("symlink %s exists but points to %s, not %s", symlinkName, target, dir)
		}
		return false, fmt.Errorf("%s already exists and is not a symlink", symlinkName)
	}

	if err := os.Symlink(dir, symlinkPath); err != nil {
		return false, fmt.Errorf("create symlink: %w", err)
	}
	return false, nil
}

// Prune deletes the top-level entries of dir older than olderThan and answers
// how many. The canvases entry is never pruned: a canvas is durable output,
// and the age of its directory says nothing about whether it is still
// wanted.
func (s *Service) Prune(dir string, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.Name() == drive.CanvasesDir {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				s.logger.Warn().Err(err).Str("path", path).Msg("failed to remove context entry during prune")
				continue
			}
			count++
		}
	}
	if count > 0 {
		s.forget(dir)
	}
	return count, nil
}

// forget drops the snapshot of the space whose directory is dir, if dir is
// one, after Init or Prune changed it behind the snapshot's back, and tells
// subscribers the whole space changed.
func (s *Service) forget(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel, err := filepath.Rel(s.cfg.ContextDir(), dir)
	if err != nil {
		return
	}
	space, err := drive.ParseSpaceKey(filepath.ToSlash(rel))
	if err != nil {
		return
	}
	delete(s.spaces, space.Key)
	s.publish(space)
}

func (s *Service) publish(space drive.Space, paths ...string) {
	s.bus.PublishDriveChanged(events.DriveChangedPayload{Space: space.Key, Paths: paths})
}

func notFound(space drive.Space, path string) error {
	return fmt.Errorf("%w: %s/%s", drive.ErrNotFound, space.Key, path)
}

// documentFile is the absolute path of a document. The caller holds s.mu.
func (s *Service) documentFile(space drive.Space, path string) (string, error) {
	if err := drive.ValidatePath(path); err != nil {
		return "", err
	}
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.FromSlash(path)), nil
}

func (s *Service) canvasFile(space drive.Space, path string) (string, error) {
	full, err := s.documentFile(space, path)
	if err != nil {
		return "", err
	}
	if drive.KindOf(path) != drive.KindCanvas {
		return "", fmt.Errorf("%w: %q is not a canvas", drive.ErrInvalidPath, path)
	}
	return full, nil
}

func (s *Service) loadCanvas(space drive.Space, path string) (drive.Canvas, bool, error) {
	full, err := s.canvasFile(space, path)
	if err != nil {
		return drive.Canvas{}, false, err
	}
	data, err := os.ReadFile(full)
	if errors.Is(err, fs.ErrNotExist) {
		return drive.Canvas{}, false, nil
	}
	if err != nil {
		return drive.Canvas{}, false, fmt.Errorf("drive: read canvas %s/%s: %w", space.Key, path, err)
	}
	var c drive.Canvas
	if err := json.Unmarshal(data, &c); err != nil {
		return drive.Canvas{}, false, fmt.Errorf("drive: parse canvas %s/%s: %w", space.Key, path, err)
	}
	// The file's own space and path fields are informational; a file moved
	// or copied by hand belongs where it sits.
	c.Space = space.Key
	c.Path = path
	if c.Frontmatter == nil {
		c.Frontmatter = drive.Frontmatter{}
	}
	if err := drive.ValidateFrontmatter(c.Frontmatter); err != nil {
		return drive.Canvas{}, false, fmt.Errorf("drive: parse canvas %s/%s: %w", space.Key, path, err)
	}
	if c.Blocks == nil {
		c.Blocks = []drive.Block{}
	}
	return c, true, nil
}

// writeCanvas replaces the canvas file, files the entry in the snapshot and
// publishes. The caller holds s.mu for writing.
func (s *Service) writeCanvas(space drive.Space, c drive.Canvas) error {
	full, err := s.canvasFile(space, c.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), dirPerm); err != nil {
		return fmt.Errorf("drive: create folder for %s/%s: %w", space.Key, c.Path, err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("drive: encode canvas %s/%s: %w", space.Key, c.Path, err)
	}
	if err := atomicfile.Write(full, data, filePerm); err != nil {
		return fmt.Errorf("drive: write canvas %s/%s: %w", space.Key, c.Path, err)
	}
	if _, err := s.refresh(space, c.Path); err != nil {
		return err
	}
	s.publish(space, c.Path)
	return nil
}

// refresh rebuilds the entry at path from disk and files it in the space's
// snapshot, with any folder above it the snapshot lacks. An unscanned space
// has no snapshot to keep current. The caller holds s.mu for writing.
func (s *Service) refresh(space drive.Space, path string) (drive.Entry, error) {
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return drive.Entry{}, err
	}
	entry, err := s.scanPath(dir, path)
	if err != nil {
		return drive.Entry{}, err
	}
	if snap, ok := s.spaces[space.Key]; ok {
		s.ensureAncestors(&snap.root, space, path)
		upsertNode(&snap.root, entry)
	}
	return cloneEntry(entry), nil
}

// ensureAncestors files every folder above path that the snapshot lacks,
// from disk. The caller holds s.mu for writing.
func (s *Service) ensureAncestors(root *drive.Entry, space drive.Space, path string) {
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return
	}
	prefix := ""
	for seg := range strings.SplitSeq(parentPath(path), "/") {
		if seg == "" {
			return
		}
		prefix = joinPath(prefix, seg)
		if findNode(root, prefix) != nil {
			continue
		}
		info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(prefix)))
		if err != nil {
			s.logger.Debug().Err(err).Str("space", space.Key).Str("path", prefix).Msg("folder missing after a write")
			return
		}
		upsertNode(root, dirEntry(prefix, info))
	}
}

// scan walks a space directory into its root entry. A directory that does
// not exist is an empty root.
func (s *Service) scan(dir string) (drive.Entry, error) {
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return drive.Entry{Kind: drive.KindDir}, nil
	}
	if err != nil {
		return drive.Entry{}, err
	}
	root := dirEntry("", info)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return drive.Entry{}, err
	}
	root.Children = s.scanEntries(dir, "", entries)
	return root, nil
}

// scanEntries indexes one directory's listing. Dot entries are skipped, as
// are symlinks and anything else that is neither a file nor a folder:
// following a link could loop, and nothing Hive writes is one.
func (s *Service) scanEntries(dir, rel string, entries []fs.DirEntry) []drive.Entry {
	if rel == drive.CanvasesDir {
		entries = s.renameLegacyCanvases(dir, entries)
	}
	children := make([]drive.Entry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			s.logger.Debug().Err(err).Str("dir", dir).Str("name", name).Msg("entry vanished during scan")
			continue
		}
		full := filepath.Join(dir, name)
		path := joinPath(rel, name)
		switch {
		case info.IsDir():
			child := dirEntry(path, info)
			sub, err := os.ReadDir(full)
			if err != nil {
				s.logger.Warn().Err(err).Str("dir", full).Msg("folder not listed")
			} else {
				child.Children = s.scanEntries(full, path, sub)
			}
			children = append(children, child)
		case info.Mode().IsRegular():
			children = append(children, s.fileEntry(full, path, info))
		}
	}
	drive.SortEntries(children)
	return children
}

// scanPath is the entry at path built from disk, with its subtree when it is
// a folder.
func (s *Service) scanPath(dir, path string) (drive.Entry, error) {
	full := filepath.Join(dir, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if err != nil {
		return drive.Entry{}, fmt.Errorf("drive: stat %s: %w", path, err)
	}
	switch {
	case info.IsDir():
		entry := dirEntry(path, info)
		sub, err := os.ReadDir(full)
		if err != nil {
			return drive.Entry{}, fmt.Errorf("drive: list %s: %w", path, err)
		}
		entry.Children = s.scanEntries(full, path, sub)
		return entry, nil
	case info.Mode().IsRegular():
		return s.fileEntry(full, path, info), nil
	default:
		return drive.Entry{}, fmt.Errorf("%w: %q is neither a document nor a folder", drive.ErrNotFound, path)
	}
}

// renameLegacyCanvases gives every canvases/<name>.json that holds a canvas
// its .canvas.json name before the listing is indexed, and answers the
// listing as it is afterwards. A target that exists is left alone with a
// warning.
func (s *Service) renameLegacyCanvases(dir string, entries []fs.DirEntry) []fs.DirEntry {
	renamed := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, drive.CanvasExt) {
			continue
		}
		src := filepath.Join(dir, name)
		data, err := os.ReadFile(src)
		if err != nil || !isCanvasJSON(data) {
			continue
		}
		dst := filepath.Join(dir, canvasStem(name)+drive.CanvasExt)
		if _, err := os.Lstat(dst); err == nil {
			s.logger.Warn().Str("file", src).Str("target", dst).Msg("legacy canvas target exists, left in place")
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			s.logger.Warn().Err(err).Str("file", src).Msg("legacy canvas not renamed")
			continue
		}
		renamed = true
	}
	if !renamed {
		return entries
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		s.logger.Warn().Err(err).Str("dir", dir).Msg("canvases folder not relisted after rename")
		return entries
	}
	return after
}

// isCanvasJSON reports whether data is a JSON object with a blocks key,
// which is what every canvas file ever written has and no other file in a
// canvases directory does.
func isCanvasJSON(data []byte) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return false
	}
	_, ok := object["blocks"]
	return ok
}

func canvasStem(filename string) string {
	stem := strings.TrimSuffix(filename, ".json")
	return strings.TrimSuffix(stem, ".canvas")
}

func (s *Service) fileEntry(full, path string, info fs.FileInfo) drive.Entry {
	entry := drive.Entry{
		Path: path, Name: baseName(path), Kind: drive.KindOf(path),
		Size: info.Size(), Modified: info.ModTime(), Created: createdAt(info),
	}
	switch entry.Kind {
	case drive.KindMarkdown:
		head, err := readHead(full, frontmatterHeadBytes)
		if err != nil {
			s.logger.Debug().Err(err).Str("file", full).Msg("markdown head not read")
			return entry
		}
		fm, present, err := drive.ParseMarkdownFrontmatter(head)
		if err != nil {
			s.logger.Debug().Err(err).Str("file", full).Msg("front matter not indexed")
		}
		if present {
			entry.Front = fm
			if title, ok := fm["title"].(string); ok {
				entry.Title = title
			}
		}
	case drive.KindCanvas:
		data, err := os.ReadFile(full)
		var c drive.Canvas
		if err == nil {
			err = json.Unmarshal(data, &c)
		}
		if err != nil {
			s.logger.Debug().Err(err).Str("file", full).Msg("canvas not indexed")
			return entry
		}
		entry.Title = c.Title
		if len(c.Frontmatter) > 0 {
			entry.Front = c.Frontmatter
		}
	default:
	}
	return entry
}

func dirEntry(path string, info fs.FileInfo) drive.Entry {
	return drive.Entry{Path: path, Name: baseName(path), Kind: drive.KindDir, Modified: info.ModTime(), Created: createdAt(info)}
}

func createdAt(info fs.FileInfo) time.Time {
	if t, ok := fsx.BirthTime(info); ok {
		return t
	}
	return info.ModTime()
}

func readHead(full string, n int) ([]byte, error) {
	f, err := os.Open(full)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, n)
	read, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	return buf[:read], nil
}

// Snapshot tree helpers. Paths are slash-separated and relative to the
// space root; "" is the root itself.

func joinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func parentPath(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

func baseName(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}

func childIndex(children []drive.Entry, name string) int {
	return slices.IndexFunc(children, func(e drive.Entry) bool { return e.Name == name })
}

// findNode is a pointer to the entry at path inside root, or nil. The
// pointer is into root's slices and is invalidated by the next insertion
// above it.
func findNode(root *drive.Entry, path string) *drive.Entry {
	if path == "" {
		return root
	}
	cur := root
	for seg := range strings.SplitSeq(path, "/") {
		i := childIndex(cur.Children, seg)
		if i < 0 {
			return nil
		}
		cur = &cur.Children[i]
	}
	return cur
}

// upsertNode files entry under its parent, replacing an entry of the same
// name. A replaced folder keeps its children when entry brings none. A
// parent the snapshot lacks is created bare; ensureAncestors runs first so
// that does not happen on a write.
func upsertNode(root *drive.Entry, entry drive.Entry) {
	parent := root
	if pp := parentPath(entry.Path); pp != "" {
		if parent = findNode(root, pp); parent == nil {
			upsertNode(root, drive.Entry{Path: pp, Name: baseName(pp), Kind: drive.KindDir})
			parent = findNode(root, pp)
		}
	}
	if i := childIndex(parent.Children, entry.Name); i >= 0 {
		if entry.Kind == drive.KindDir && entry.Children == nil {
			entry.Children = parent.Children[i].Children
		}
		parent.Children[i] = entry
	} else {
		parent.Children = append(parent.Children, entry)
	}
	drive.SortEntries(parent.Children)
}

func removeNode(root *drive.Entry, path string) (drive.Entry, bool) {
	parent := findNode(root, parentPath(path))
	if parent == nil {
		return drive.Entry{}, false
	}
	i := childIndex(parent.Children, baseName(path))
	if i < 0 {
		return drive.Entry{}, false
	}
	removed := parent.Children[i]
	parent.Children = slices.Delete(parent.Children, i, i+1)
	return removed, true
}

// rekey moves entry and its subtree to path.
func rekey(entry *drive.Entry, path string) {
	entry.Path = path
	entry.Name = baseName(path)
	for i := range entry.Children {
		rekey(&entry.Children[i], path+"/"+entry.Children[i].Name)
	}
}

// flatten is every entry below root in tree order, root excluded.
func flatten(root drive.Entry) []drive.Entry {
	var out []drive.Entry
	var walk func(e drive.Entry)
	walk = func(e drive.Entry) {
		for _, child := range e.Children {
			out = append(out, child)
			walk(child)
		}
	}
	walk(root)
	return out
}

func cloneEntry(entry drive.Entry) drive.Entry {
	entry.Front = cloneFrontmatter(entry.Front)
	if entry.Children != nil {
		children := make([]drive.Entry, len(entry.Children))
		for i, child := range entry.Children {
			children[i] = cloneEntry(child)
		}
		entry.Children = children
	}
	return entry
}

func cloneFrontmatter(fm drive.Frontmatter) drive.Frontmatter {
	if fm == nil {
		return nil
	}
	cloned := make(drive.Frontmatter, len(fm))
	for key, value := range fm {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(value))
		for key, item := range value {
			cloned[key] = cloneValue(item)
		}
		return cloned
	case drive.Frontmatter:
		return cloneFrontmatter(value)
	case []any:
		cloned := make([]any, len(value))
		for i, item := range value {
			cloned[i] = cloneValue(item)
		}
		return cloned
	default:
		return value
	}
}
