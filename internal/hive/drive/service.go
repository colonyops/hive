// Package drive is the Hive Drive service: the document tree under the
// context root, served from one in-memory snapshot per space. A space is
// scanned the first time something reads it, and every write through the
// service keeps its snapshot current and publishes drive.changed.
package drive

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/config"
	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/hive/events"
	"github.com/colonyops/hive/internal/platform/fswatch"
	"github.com/colonyops/hive/internal/platform/git"
	"github.com/colonyops/hive/pkg/atomicfile"
	"github.com/colonyops/hive/pkg/fsx"
	"github.com/colonyops/hive/pkg/logutils"
)

const (
	// frontmatterHeadBytes bounds what a scan reads from a markdown file:
	// 8 KiB holds any front matter written by hand or by an agent, and keeps
	// a scan to one small read per file. Front matter past it is not indexed,
	// and the file is not an error.
	frontmatterHeadBytes = 8 * 1024
	defaultSearchLimit   = 50
	dirPerm              = 0o755
	filePerm             = 0o644

	// finderMetadata is the file Finder drops into every folder the user
	// opens. A folder holding only it looks empty, so Delete removes it.
	finderMetadata = ".DS_Store"
)

type Hit struct {
	Space drive.Space `json:"space"`
	Entry drive.Entry `json:"entry"`
}

type spaceSnapshot struct {
	root drive.Entry
}

// Service serves the drive. Tree and Search of a scanned space are served
// from the snapshot; a mutation does its file operation, files the result in
// the snapshot, and publishes once with the author it was given.
type Service struct {
	logger zerolog.Logger
	// baseLogger carries no component label, so the watcher can add its own.
	baseLogger zerolog.Logger
	bus        *events.EventBus
	now        func() time.Time

	mu     sync.RWMutex
	cfg    *config.Config
	git    git.Git
	spaces map[string]*spaceSnapshot
	// generation counts the changes that could land in a space while it is
	// scanned outside s.mu; see scanned.
	generation uint64

	debounce time.Duration
	rootPoll time.Duration
	watch    *watcher
	watchCtx context.Context
	// watchWanted survives a watcher that failed to start, so a Reconfigure
	// to a usable root starts one again.
	watchWanted bool
	watchers    sync.WaitGroup
}

func NewService(logger zerolog.Logger, cfg *config.Config, gitClient git.Git, bus *events.EventBus) *Service {
	return &Service{
		logger:     logutils.Component(logger, "drive"),
		baseLogger: logger,
		bus:        bus,
		now:        time.Now,
		cfg:        cfg,
		git:        gitClient,
		spaces:     map[string]*spaceSnapshot{},
		debounce:   fswatch.DefaultDebounce,
		rootPoll:   defaultRootPoll,
	}
}

// Reconfigure is what Engine.Reload calls. The snapshots and the watcher
// describe one root, so a changed ContextDir drops the snapshots and moves a
// running watcher; anything else only swaps the config and the git executor
// built for it. Every space that had a snapshot is published as changed
// whole, because its documents now come from another directory.
func (s *Service) Reconfigure(cfg *config.Config, gitClient git.Git) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rerooted := cfg.ContextDir() != s.cfg.ContextDir()
	var dropped []string
	if rerooted {
		dropped = slices.Sorted(maps.Keys(s.spaces))
		s.spaces = map[string]*spaceSnapshot{}
		s.generation++
	}
	s.cfg = cfg
	s.git = gitClient
	if rerooted && s.watchWanted {
		s.restartWatcher()
	}
	for _, key := range dropped {
		s.publish(drive.Space{Key: key}, drive.Author{})
	}
}

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
	owners, err := s.subdirectories(root)
	if err != nil {
		return nil, fmt.Errorf("drive: list spaces: %w", err)
	}
	var repositories, workspaces []drive.Space
	for _, owner := range owners {
		switch {
		case owner == drive.WorkspacesDir:
			names, err := s.subdirectories(filepath.Join(root, owner))
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
			repos, err := s.subdirectories(filepath.Join(root, owner))
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
func (s *Service) subdirectories(dir string) ([]string, error) {
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
		info, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil {
			s.logger.Debug().Err(err).Str("dir", dir).Str("name", entry.Name()).Msg("entry not listed as a space")
			continue
		}
		if info.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// SpaceForRemote is the repository space a git remote URL belongs to, keyed
// as CanonicalSpace spells it.
func (s *Service) SpaceForRemote(remote string) (drive.Space, bool) {
	space, ok := SpaceForRemote(remote)
	if !ok {
		return drive.Space{}, false
	}
	return s.CanonicalSpace(space), true
}

// SpaceForRemote keeps the remote's spelling, which on a case-insensitive
// file system may not be the key the service uses; prefer the method.
func SpaceForRemote(remote string) (drive.Space, bool) {
	owner, repo := git.ExtractOwnerRepo(remote)
	if owner == "" || repo == "" {
		return drive.Space{}, false
	}
	space, err := drive.RepositorySpace(owner, repo)
	return space, err == nil
}

// CanonicalSpace respells a repository space's key as the directories on
// disk spell it, when those directories exist. On a case-insensitive file
// system ACME/site and acme/site are one directory, but the snapshots and the
// watcher key a space by its on-disk name, so a space resolved from a remote
// URL or a --repo flag must use that name to see changes made elsewhere.
func (s *Service) CanonicalSpace(space drive.Space) drive.Space {
	if space.Kind != drive.SpaceKindRepository {
		return space
	}
	owner, repo, ok := strings.Cut(space.Key, "/")
	if !ok {
		return space
	}
	root := s.Root()
	owner = s.onDiskName(root, owner)
	repo = s.onDiskName(filepath.Join(root, owner), repo)
	canonical, err := drive.RepositorySpace(owner, repo)
	if err != nil {
		s.logger.Debug().Err(err).Str("space", space.Key).Msg("on-disk spelling is no repository space")
		return space
	}
	return canonical
}

// onDiskName is how dir's listing spells name: name itself when an entry has
// exactly that name, otherwise the entry matching it case-insensitively, but
// only when name resolves to an entry at all. On a case-sensitive file system
// a name that does not exist is a different, new entry and keeps its case.
func (s *Service) onDiskName(dir, name string) string {
	if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.logger.Debug().Err(err).Str("dir", dir).Str("name", name).Msg("entry spelling not checked")
		}
		return name
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.logger.Debug().Err(err).Str("dir", dir).Msg("folder not listed for an entry's spelling")
		return name
	}
	match := name
	for _, entry := range entries {
		if entry.Name() == name {
			return name
		}
		if match == name && strings.EqualFold(entry.Name(), name) {
			match = entry.Name()
		}
	}
	return match
}

// onDiskPath is path below dir with every component spelled as on disk.
func (s *Service) onDiskPath(dir, path string) string {
	out := ""
	for seg := range strings.SplitSeq(path, "/") {
		name := s.onDiskName(dir, seg)
		out = joinPath(out, name)
		dir = filepath.Join(dir, name)
	}
	return out
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

// SpaceAt is the space holding path and path's location inside it, when
// path sits under the root. Both are resolved through symbolic links first,
// so a document reached through a checkout's .hive link is found.
func (s *Service) SpaceAt(path string) (drive.Space, string, bool) {
	root, err := resolvePath(s.Root())
	if err != nil {
		return drive.Space{}, "", false
	}
	resolved, err := resolvePath(path)
	if err != nil {
		return drive.Space{}, "", false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(rel) {
		return drive.Space{}, "", false
	}
	space, inner, err := drive.SplitSpacePath(filepath.ToSlash(rel))
	return space, inner, err == nil
}

func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
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
		space, err := drive.RepositorySpace(owner, name)
		if err != nil {
			return drive.Space{}, err
		}
		return s.CanonicalSpace(space), nil
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

// Tree is the space's whole tree, scanned on first use. The result is a deep
// copy: a caller may keep or change it without touching the snapshot. A space
// whose directory does not exist is an empty root, not an error.
func (s *Service) Tree(_ context.Context, space drive.Space) (drive.Entry, error) {
	snap, err := s.scanned(space)
	if err != nil {
		return drive.Entry{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneEntry(snap.root), nil
}

// Entry is the entry at path in the space's snapshot, with its subtree, as a
// deep copy; path "" is the space root. The space is scanned on first use.
// An entry the snapshot does not hold is ErrNotFound.
func (s *Service) Entry(_ context.Context, space drive.Space, path string) (drive.Entry, error) {
	if path != "" {
		if err := drive.ValidatePath(path); err != nil {
			return drive.Entry{}, err
		}
	}
	snap, err := s.scanned(space)
	if err != nil {
		return drive.Entry{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return entryAt(snap, space, path)
}

func entryAt(snap *spaceSnapshot, space drive.Space, path string) (drive.Entry, error) {
	node := findNode(&snap.root, path)
	if node == nil {
		return drive.Entry{}, notFound(space, path)
	}
	return cloneEntry(*node), nil
}

// Locate is the absolute path of a document or folder in a space, whether or
// not it exists; path "" is the space's own directory.
func (s *Service) Locate(space drive.Space, path string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if path == "" {
		return spaceDir(s.cfg.ContextDir(), space)
	}
	return s.documentFile(space, path)
}

// scanned answers the space's snapshot, scanning on first use. The scan
// head-reads every document, so it runs without s.mu, keeping readers of
// other spaces unblocked. A write or a watched change that lands meanwhile
// bumps s.generation, and the scan is then redone under the lock so the
// snapshot cannot miss it. The caller holds no lock; the snapshot is read
// under s.mu.
func (s *Service) scanned(space drive.Space) (*spaceSnapshot, error) {
	s.mu.RLock()
	snap, ok := s.spaces[space.Key]
	s.mu.RUnlock()
	if ok {
		return snap, nil
	}

	s.mu.Lock()
	if snap, ok := s.spaces[space.Key]; ok {
		s.mu.Unlock()
		return snap, nil
	}
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	generation := s.generation
	s.mu.Unlock()

	root, scanErr := s.scan(dir)

	s.mu.Lock()
	defer s.mu.Unlock()
	if snap, ok := s.spaces[space.Key]; ok {
		return snap, nil
	}
	if scanErr != nil || s.generation != generation {
		return s.snapshot(space)
	}
	snap = &spaceSnapshot{root: root}
	s.spaces[space.Key] = snap
	return snap, nil
}

// snapshot is scanned with s.mu held for writing throughout.
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

// Search splits query into words and answers the entries in every space
// whose path or title holds each word, case-insensitively; a hyphen or an
// underscore matches a space, so "session lifecycle" finds
// 2026-06-12-session-lifecycle.md. Spaces no one has read yet are scanned,
// in order, until limit is reached. Hits come grouped in Spaces order and
// sorted by path inside a space, without children; limit caps them, and 0
// means defaultSearchLimit. An empty query matches nothing.
func (s *Service) Search(ctx context.Context, query string, limit int) (hits []Hit, err error) {
	defer recordOp(ctx, opSearch, actorNone, time.Now(), &err)
	defer func() { trace.SpanFromContext(ctx).SetAttributes(attribute.Int(attrHits, len(hits))) }()
	terms := strings.Fields(searchText(query))
	if len(terms) == 0 {
		return []Hit{}, nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	spaces, err := s.Spaces(ctx)
	if err != nil {
		return nil, err
	}

	hits = []Hit{}
	for _, space := range spaces {
		snap, err := s.scanned(space)
		if err != nil {
			return nil, err
		}
		s.mu.RLock()
		found := searchSnapshot(snap, space, terms)
		s.mu.RUnlock()
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

func searchSnapshot(snap *spaceSnapshot, space drive.Space, terms []string) []Hit {
	var found []Hit
	for _, entry := range flatten(snap.root) {
		if !matchesEvery(entry, terms) {
			continue
		}
		entry.Children = nil
		found = append(found, Hit{Space: space, Entry: cloneEntry(entry)})
	}
	slices.SortFunc(found, func(a, b Hit) int { return strings.Compare(a.Entry.Path, b.Entry.Path) })
	return found
}

// Recent answers the documents in every space, newest created first, without
// children; folders are left out. Ties fall back to the newest modified, then
// to the space and the path, so the order is stable. limit caps them, and 0
// means defaultSearchLimit.
func (s *Service) Recent(ctx context.Context, limit int) (hits []Hit, err error) {
	defer recordOp(ctx, opRecent, actorNone, time.Now(), &err)
	defer func() { trace.SpanFromContext(ctx).SetAttributes(attribute.Int(attrHits, len(hits))) }()
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	spaces, err := s.Spaces(ctx)
	if err != nil {
		return nil, err
	}

	hits = []Hit{}
	for _, space := range spaces {
		snap, err := s.scanned(space)
		if err != nil {
			return nil, err
		}
		s.mu.RLock()
		for _, entry := range flatten(snap.root) {
			if entry.Kind == drive.KindDir {
				continue
			}
			hits = append(hits, Hit{Space: space, Entry: cloneEntry(entry)})
		}
		s.mu.RUnlock()
	}
	slices.SortFunc(hits, func(a, b Hit) int {
		return cmp.Or(
			b.Entry.Created.Compare(a.Entry.Created),
			b.Entry.Modified.Compare(a.Entry.Modified),
			strings.Compare(a.Space.Key, b.Space.Key),
			strings.Compare(a.Entry.Path, b.Entry.Path),
		)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// matchesEvery separates path and title with a newline so no word matches
// across the two.
func matchesEvery(entry drive.Entry, terms []string) bool {
	haystack := searchText(entry.Path) + "\n" + searchText(entry.Title)
	for _, term := range terms {
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

var searchSeparators = strings.NewReplacer("-", " ", "_", " ")

func searchText(text string) string {
	return strings.ToLower(searchSeparators.Replace(text))
}

// Read is the entry at path, built from the file as it is now, and the file's
// bytes. A folder is not a document and is ErrInvalidPath.
func (s *Service) Read(ctx context.Context, space drive.Space, path string) (_ drive.Entry, _ []byte, err error) {
	defer recordOp(ctx, opRead, actorNone, time.Now(), &err)
	annotate(ctx, space, path)
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
// above it. Anything else is ErrNotWritable.
func (s *Service) Write(ctx context.Context, space drive.Space, path string, author drive.Author, body []byte) (_ drive.Entry, err error) {
	defer recordOp(ctx, opWrite, actorOf(author), time.Now(), &err)
	annotate(ctx, space, path, attribute.Int(attrBytes, len(body)))
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
	s.logger.Debug().Ctx(ctx).Str("space", space.Key).Str("path", entry.Path).Int("bytes", len(body)).
		Str("session", author.Session).Str("hive_session", author.HiveSession).Msg("document written")
	s.publish(space, author, entry.Path)
	return entry, nil
}

// CreateFolder makes one folder, and the folders above it. A path that
// exists, as anything, is ErrExists.
func (s *Service) CreateFolder(ctx context.Context, space drive.Space, path string, author drive.Author) (_ drive.Entry, err error) {
	defer recordOp(ctx, opCreateFolder, actorOf(author), time.Now(), &err)
	annotate(ctx, space, path)
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
	s.publish(space, author, entry.Path)
	return entry, nil
}

// Move renames a document or a folder inside its space, creating the folders
// above the target. The target must not exist (ErrExists), and a folder
// cannot move into itself (ErrInvalidPath). A document keeps its kind. A
// rename that changes only case is allowed on a case-insensitive file system,
// where the target is the source itself. A moved folder's descendants are
// re-keyed in the snapshot rather than rescanned.
func (s *Service) Move(ctx context.Context, space drive.Space, from, to string, author drive.Author) (_ drive.Entry, err error) {
	defer recordOp(ctx, opMove, actorOf(author), time.Now(), &err)
	annotate(ctx, space, from, attribute.String(attrTarget, to))
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
	srcInfo, err := os.Lstat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return drive.Entry{}, notFound(space, from)
	} else if err != nil {
		return drive.Entry{}, fmt.Errorf("drive: move %s/%s: %w", space.Key, from, err)
	}
	if !srcInfo.IsDir() {
		if err := checkKeepsKind(from, to); err != nil {
			return drive.Entry{}, err
		}
	}
	if dstInfo, err := os.Lstat(dst); err == nil {
		if !os.SameFile(srcInfo, dstInfo) {
			return drive.Entry{}, fmt.Errorf("%w: %q", drive.ErrExists, to)
		}
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
	s.logger.Info().Ctx(ctx).Str("space", space.Key).Str("from", from).Str("to", to).
		Str("session", author.Session).Str("hive_session", author.HiveSession).Msg("drive entry moved")
	s.publish(space, author, from, to)
	return entry, nil
}

// checkKeepsKind refuses a document move that would hand one renderer the
// other's source.
func checkKeepsKind(from, to string) error {
	if drive.KindOf(from) != drive.KindOf(to) {
		return fmt.Errorf("%w: moving %q to %q would change its kind; keep the extension", drive.ErrInvalidPath, from, to)
	}
	return nil
}

// Delete removes a document or an empty folder. A folder with anything in
// it but Finder's .DS_Store, hidden files included, is ErrNotEmpty.
func (s *Service) Delete(ctx context.Context, space drive.Space, path string, author drive.Author) (err error) {
	defer recordOp(ctx, opDelete, actorOf(author), time.Now(), &err)
	annotate(ctx, space, path)
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
		if err := clearEmptyFolder(full); err != nil {
			if errors.Is(err, drive.ErrNotEmpty) {
				return fmt.Errorf("%w: %q", drive.ErrNotEmpty, path)
			}
			return fmt.Errorf("drive: delete %s/%s: %w", space.Key, path, err)
		}
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("drive: delete %s/%s: %w", space.Key, path, err)
	}
	if snap, ok := s.spaces[space.Key]; ok {
		removeNode(&snap.root, path)
	}
	s.logger.Info().Ctx(ctx).Str("space", space.Key).Str("path", path).Bool("folder", info.IsDir()).
		Str("session", author.Session).Str("hive_session", author.HiveSession).Msg("drive entry deleted")
	s.publish(space, author, path)
	return nil
}

// clearEmptyFolder removes the finderMetadata file from a folder that holds
// nothing else, and is ErrNotEmpty for a folder that holds anything else.
func clearEmptyFolder(full string) error {
	entries, err := os.ReadDir(full)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != finderMetadata || entry.IsDir() {
			return drive.ErrNotEmpty
		}
	}
	if len(entries) == 0 {
		return nil
	}
	return os.Remove(filepath.Join(full, finderMetadata))
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
			target, err := os.Readlink(symlinkPath)
			if err != nil {
				return false, fmt.Errorf("read symlink %s: %w", symlinkName, err)
			}
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
// how many.
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
		info, err := entry.Info()
		if err != nil {
			s.logger.Debug().Err(err).Str("dir", dir).Str("name", entry.Name()).Msg("entry vanished before prune")
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
		s.logger.Debug().Err(err).Str("dir", dir).Msg("changed folder is not under the root")
		return
	}
	space, err := drive.ParseSpaceKey(filepath.ToSlash(rel))
	if err != nil {
		s.logger.Debug().Err(err).Str("dir", dir).Msg("changed folder is not a space directory")
		return
	}
	delete(s.spaces, space.Key)
	s.publish(space, drive.Author{})
}

// publish is called with s.mu held for writing.
func (s *Service) publish(space drive.Space, author drive.Author, paths ...string) {
	s.generation++
	s.bus.PublishDriveChanged(events.DriveChangedPayload{Space: space.Key, Paths: paths, Author: author})
}

func notFound(space drive.Space, path string) error {
	return fmt.Errorf("%w: %s/%s", drive.ErrNotFound, space.Key, path)
}

// documentFile is the absolute path of a document. A symbolic link below
// the space directory is refused, since following one reads or writes
// outside the root and the scan never lists one. The space directory itself
// may be reached through a link. The caller holds s.mu.
func (s *Service) documentFile(space drive.Space, path string) (string, error) {
	if err := drive.ValidatePath(path); err != nil {
		return "", err
	}
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return "", err
	}
	if err := refuseLinks(dir, path); err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.FromSlash(path)), nil
}

// refuseLinks stops at the first component of path that does not exist:
// whatever a write creates below it is a plain folder.
func refuseLinks(dir, path string) error {
	for seg := range strings.SplitSeq(path, "/") {
		dir = filepath.Join(dir, seg)
		info, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("drive: stat %s: %w", path, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: %q passes through a symbolic link", drive.ErrInvalidPath, path)
		}
	}
	return nil
}

// refresh rebuilds the entry at path from disk and files it in the space's
// snapshot, with any folder above it the snapshot lacks. The entry takes the
// spelling on disk, which on a case-insensitive file system can differ from
// path's. An unscanned space has no snapshot to keep current. The caller
// holds s.mu for writing.
func (s *Service) refresh(space drive.Space, path string) (drive.Entry, error) {
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return drive.Entry{}, err
	}
	path = s.onDiskPath(dir, path)
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
	start := time.Now()
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
	n := recordScan(start, root)
	s.logger.Debug().Str("dir", dir).Int("entries", n).Dur("took", time.Since(start)).Msg("space scanned")
	return root, nil
}

// scanEntries indexes one directory's listing. Dot entries are skipped, as
// are symlinks and anything else that is neither a file nor a folder:
// following a link could loop, and nothing Hive writes is one.
func (s *Service) scanEntries(dir, rel string, entries []fs.DirEntry) []drive.Entry {
	children := make([]drive.Entry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if skipEntry(name) {
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

// skipEntry is the one rule for what a listing leaves out: dot entries and
// the files hive kept for itself.
func skipEntry(name string) bool {
	return strings.HasPrefix(name, ".") || drive.IsHiveState(name)
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
	case drive.KindDir, drive.KindHtml, drive.KindFile:
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
