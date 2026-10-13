package drive

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/colonyops/hive/internal/domain/drive"
	"github.com/colonyops/hive/internal/platform/fswatch"
)

// defaultRootPoll is how often a watcher whose root vanished checks for the
// root to come back.
const defaultRootPoll = 250 * time.Millisecond

// watcher is one fswatch over the root the service had when it started, and
// the goroutine draining it. A Reconfigure to another root retires it.
type watcher struct {
	fs      *fswatch.Watcher
	cfgRoot string // ContextDir() as configured
	absRoot string // what Change.Dirs are reported under
}

// Watch keeps the snapshots in step with the disk: a document written by an
// editor, by an agent through a checkout's .hive link, or by another hive
// process lands in its space's snapshot and publishes drive.changed as if
// the service wrote it. The root is created first, because a fresh install
// has none yet. If the root is later removed, the watcher waits for it to be
// created again and then rescans every scanned space. The watcher runs until
// ctx is done or Close is called, and a second Watch is a no-op. A watcher
// that fails to start, here or after a Reconfigure, is started again by the
// next Reconfigure to another root.
func (s *Service) Watch(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watch != nil {
		return nil
	}
	s.watchWanted = true
	s.watchCtx = ctx
	w, err := s.startWatcher(ctx, s.cfg.ContextDir())
	if err != nil {
		return err
	}
	s.watch = w
	return nil
}

// Close stops the watcher and waits for its goroutine. It is idempotent and
// a no-op when Watch never ran.
func (s *Service) Close() error {
	s.mu.Lock()
	w := s.watch
	s.watch = nil
	s.watchWanted = false
	s.mu.Unlock()

	var err error
	if w != nil {
		err = w.fs.Close()
	}
	s.watchers.Wait()
	return err
}

// startWatcher is called with s.mu held for writing.
func (s *Service) startWatcher(ctx context.Context, root string) (*watcher, error) {
	if err := os.MkdirAll(root, dirPerm); err != nil {
		return nil, fmt.Errorf("drive: create root %s: %w", root, err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("drive: watch %s: %w", root, err)
	}
	fsw, err := fswatch.New(s.baseLogger, root, fswatch.WithDebounce(s.debounce))
	if err != nil {
		return nil, fmt.Errorf("drive: watch %s: %w", root, err)
	}
	w := &watcher{fs: fsw, cfgRoot: root, absRoot: absRoot}
	s.watchers.Add(1)
	go s.drain(ctx, w)
	return w, nil
}

// restartWatcher moves the watcher, running or not, to the current root.
// The caller holds s.mu for writing. The retired watcher's goroutine ends on
// its own once its channel closes; it is not waited for here because it may
// be waiting for s.mu.
func (s *Service) restartWatcher() {
	if old := s.watch; old != nil {
		s.watch = nil
		if err := old.fs.Close(); err != nil {
			s.logger.Debug().Err(err).Str("root", old.cfgRoot).Msg("previous drive watcher not closed cleanly")
		}
	}
	if s.watchCtx.Err() != nil {
		s.watchWanted = false
		return
	}
	w, err := s.startWatcher(s.watchCtx, s.cfg.ContextDir())
	if err != nil {
		s.logger.Warn().Err(err).Str("root", s.cfg.ContextDir()).Msg("drive watcher not restarted after the context directory moved")
		return
	}
	s.logger.Info().Str("root", s.cfg.ContextDir()).Msg("drive watcher moved to the new context directory")
	s.watch = w
}

func (s *Service) drain(ctx context.Context, w *watcher) {
	defer s.watchers.Done()
	defer s.retire(ctx, w)
	for {
		select {
		case <-ctx.Done():
			return
		case change, ok := <-w.fs.Changes():
			if !ok {
				s.awaitRoot(ctx, w)
				return
			}
			s.apply(ctx, w, change)
		}
	}
}

// retire runs when w's goroutine ends. If w is still the service's watcher,
// only ctx can have ended it, so the service stops watching and a later Watch
// starts afresh; a Reconfigure cannot restart on a dead context.
func (s *Service) retire(ctx context.Context, w *watcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watch == w {
		s.watch = nil
		if err := w.fs.Close(); err != nil {
			s.logger.Debug().Err(err).Str("root", w.cfgRoot).Msg("drive watcher not closed cleanly")
		}
	}
	if ctx.Err() != nil && s.watchCtx == ctx {
		s.watchWanted = false
	}
}

// awaitRoot runs after w's channel closed. When w is still the service's
// watcher, nothing retired it, so the root went away: awaitRoot polls until
// the root is a directory again, then starts a fresh watcher over it and
// rescans every scanned space, since anything may have changed meanwhile.
// It gives up as soon as Close, a Reconfigure, or ctx retires w.
func (s *Service) awaitRoot(ctx context.Context, w *watcher) {
	ticker := time.NewTicker(s.rootPoll)
	defer ticker.Stop()
	for !s.restartOnRoot(ctx, w) {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// restartOnRoot reports whether awaitRoot is done: w was retired, or the
// root is back and a new watcher runs over it.
func (s *Service) restartOnRoot(ctx context.Context, w *watcher) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watch != w {
		return true
	}
	if info, err := os.Stat(w.cfgRoot); err != nil || !info.IsDir() {
		return false
	}
	next, err := s.startWatcher(ctx, w.cfgRoot)
	if err != nil {
		s.logger.Warn().Err(err).Str("root", w.cfgRoot).Msg("drive watcher not restarted after the root came back")
		return false
	}
	s.watch = next
	s.logger.Info().Str("root", w.cfgRoot).Msg("drive watcher restarted after the root came back")
	s.resyncSpaces()
	return true
}

// spaceDelta is what one Change did to a space.
type spaceDelta struct {
	space drive.Space
	paths map[string]struct{}
	whole bool
}

// apply re-reads the directories a Change names into the snapshots of the
// spaces they sit in and publishes once per space that changed. A space no
// one has read yet is skipped: it is scanned whole on first use. The root
// and an owner directory hold spaces rather than documents, and Spaces reads
// the disk, so a change there only reconciles the scanned spaces. A resync
// rescans every scanned space whole.
func (s *Service) apply(ctx context.Context, w *watcher, change fswatch.Change) {
	// A change on disk is a cause entering the app, so it roots its own
	// trace rather than joining whatever span ctx carried at Watch.
	ctx, span := tracer.Start(ctx, "drive.watch change", trace.WithNewRoot(), trace.WithAttributes(
		attribute.Int(attrWatchDirs, len(change.Dirs)),
		attribute.Bool(attrWatchResync, change.Resync),
	))
	defer span.End()

	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation++
	if s.cfg.ContextDir() != w.cfgRoot {
		return
	}
	if change.Resync {
		watchChanges.Add(ctx, 1, watchAttrs[watchResync])
		s.resyncSpaces()
		return
	}

	deltas := map[string]*spaceDelta{}
	record := func(space drive.Space, paths []string, whole bool) {
		if len(paths) == 0 && !whole {
			return
		}
		d := deltas[space.Key]
		if d == nil {
			d = &spaceDelta{space: space, paths: map[string]struct{}{}}
			deltas[space.Key] = d
		}
		d.whole = d.whole || whole
		for _, p := range paths {
			d.paths[p] = struct{}{}
		}
	}

	aboveSpaces := false
	for _, dir := range change.Dirs {
		rel, err := filepath.Rel(w.absRoot, dir)
		if err != nil || !filepath.IsLocal(rel) {
			continue
		}
		space, path, err := drive.SplitSpacePath(filepath.ToSlash(rel))
		if err != nil {
			aboveSpaces = true
			continue
		}
		snap, ok := s.spaces[space.Key]
		if !ok {
			continue
		}
		paths, whole := s.rescanDir(snap, space, path)
		record(space, paths, whole)
	}
	if aboveSpaces {
		s.reconcileSpaces(record)
	}

	span.SetAttributes(attribute.Int(attrWatchSpaces, len(deltas)))
	if len(deltas) == 0 {
		watchChanges.Add(ctx, 1, watchAttrs[watchUnchanged])
		return
	}
	watchChanges.Add(ctx, 1, watchAttrs[watchPublished])
	s.logger.Debug().Ctx(ctx).Int("dirs", len(change.Dirs)).Strs("spaces", slices.Sorted(maps.Keys(deltas))).
		Msg("documents changed outside the drive")

	for _, key := range slices.Sorted(maps.Keys(deltas)) {
		d := deltas[key]
		if d.whole {
			s.publish(d.space, drive.Author{})
			continue
		}
		s.publish(d.space, drive.Author{}, slices.Sorted(maps.Keys(d.paths))...)
	}
}

// resyncSpaces rescans every scanned space from disk and publishes each as
// changed whole. A space that no longer scans loses its snapshot and is
// scanned again on next use. The caller holds s.mu for writing.
func (s *Service) resyncSpaces() {
	for _, key := range slices.Sorted(maps.Keys(s.spaces)) {
		space, err := drive.ParseSpaceKey(key)
		if err != nil {
			delete(s.spaces, key)
			continue
		}
		dir, err := spaceDir(s.cfg.ContextDir(), space)
		if err == nil {
			var root drive.Entry
			if root, err = s.scan(dir); err == nil {
				s.spaces[key] = &spaceSnapshot{root: root}
			}
		}
		if err != nil {
			s.logger.Warn().Err(err).Str("space", key).Msg("space not rescanned after lost events")
			delete(s.spaces, key)
		}
		s.publish(space, drive.Author{})
	}
}

// rescanDir re-reads one folder of a scanned space into its snapshot and
// answers the paths whose entries changed: a document that is new or
// rewritten, a folder new to the snapshot, or an entry that vanished. A new
// folder is walked whole, because a rename or a move raises one event on
// the parent and none below it. whole reports that the space directory
// itself is gone.
func (s *Service) rescanDir(snap *spaceSnapshot, space drive.Space, rel string) (changed []string, whole bool) {
	dir, err := spaceDir(s.cfg.ContextDir(), space)
	if err != nil {
		return nil, false
	}
	full := filepath.Join(dir, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		if rel == "" {
			return nil, resetSnapshot(snap)
		}
		if _, removed := removeNode(&snap.root, rel); removed {
			return []string{rel}, false
		}
		return nil, false
	}

	node := findNode(&snap.root, rel)
	if node == nil {
		entry, err := s.scanPath(dir, rel)
		if err != nil {
			s.logger.Warn().Err(err).Str("space", space.Key).Str("path", rel).Msg("changed folder not rescanned")
			return nil, false
		}
		s.ensureAncestors(&snap.root, space, rel)
		upsertNode(&snap.root, entry)
		return []string{rel}, false
	}

	entries, err := os.ReadDir(full)
	if err != nil {
		s.logger.Warn().Err(err).Str("space", space.Key).Str("path", rel).Msg("changed folder not listed")
		return nil, false
	}
	previous := make(map[string]drive.Entry, len(node.Children))
	for _, child := range node.Children {
		previous[child.Name] = child
	}
	next := make([]drive.Entry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if skipEntry(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			s.logger.Debug().Err(err).Str("space", space.Key).Str("path", joinPath(rel, name)).Msg("entry vanished during rescan")
			continue
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		path := joinPath(rel, name)
		prev, known := previous[name]
		delete(previous, name)
		switch {
		case info.IsDir():
			if known && prev.Kind == drive.KindDir {
				next = append(next, prev)
				continue
			}
			sub, err := s.scanPath(dir, path)
			if err != nil {
				s.logger.Warn().Err(err).Str("space", space.Key).Str("path", path).Msg("new folder not indexed")
				if known {
					changed = append(changed, path)
				}
				continue
			}
			next = append(next, sub)
		case known && prev.Kind != drive.KindDir && prev.Size == info.Size() && prev.Modified.Equal(info.ModTime()):
			next = append(next, prev)
			continue
		default:
			next = append(next, s.fileEntry(filepath.Join(full, name), path, info))
		}
		changed = append(changed, path)
	}
	for name := range previous {
		changed = append(changed, joinPath(rel, name))
	}
	drive.SortEntries(next)
	node.Children = next
	node.Modified = info.ModTime()
	node.Created = createdAt(info)
	return changed, false
}

// reconcileSpaces re-reads the root of every scanned space after a change
// above the spaces. A space directory that appeared or vanished is reported
// through its parent only: a removed directory is never named, and one
// created before its parent was watched raised no event of its own.
func (s *Service) reconcileSpaces(record func(drive.Space, []string, bool)) {
	for key, snap := range s.spaces {
		space, err := drive.ParseSpaceKey(key)
		if err != nil {
			continue
		}
		paths, whole := s.rescanDir(snap, space, "")
		record(space, paths, whole)
	}
}

// resetSnapshot puts snap in the form scan gives a space with no directory
// and reports whether that changed anything.
func resetSnapshot(snap *spaceSnapshot) bool {
	if len(snap.root.Children) == 0 && snap.root.Modified.IsZero() {
		return false
	}
	snap.root = drive.Entry{Kind: drive.KindDir}
	return true
}
