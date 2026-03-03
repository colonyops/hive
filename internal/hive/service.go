package hive

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/colonyops/hive/internal/core/action"
	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/core/eventbus"
	"github.com/colonyops/hive/internal/core/git"
	"github.com/colonyops/hive/internal/core/messaging"
	"github.com/colonyops/hive/internal/core/session"
	coretmux "github.com/colonyops/hive/internal/core/tmux"
	"github.com/colonyops/hive/pkg/executil"
	"github.com/colonyops/hive/pkg/randid"
	"github.com/colonyops/hive/pkg/tmpl"
	"github.com/rs/zerolog"
)

// CreateOptions configures session creation.
type CreateOptions struct {
	Name          string // Session name (used in path)
	SessionID     string // Session ID (auto-generated if empty)
	Prompt        string // Prompt to pass to spawned terminal (batch only)
	Remote        string // Git remote URL to clone (auto-detected if empty)
	Source        string // Source directory for file copying
	UseBatchSpawn bool   // Use batch_spawn commands instead of spawn
	Background    bool   // Create session without attaching to tmux
	// CloneStrategy selects the clone method: "full" (default) or "worktree".
	// Empty resolves via config rule matching, then global config, then "full".
	CloneStrategy string
	// SkipSpawn skips the configured spawn strategy (spawn: / batch_spawn: / windows:).
	// The caller is responsible for launching any terminal or tmux session. Use this
	// when the session directory is needed but terminal management happens elsewhere
	// (e.g. CreateSessionWithWindows, which creates the tmux session itself).
	SkipSpawn bool
	// Progress receives human-readable progress lines during session creation.
	// When non-nil, service output (hooks, file copies) is also redirected here.
	Progress io.Writer
}

// switchWriter is an io.Writer whose target can be swapped at runtime.
// All sub-components share a pointer to the same switchWriter, so redirecting
// it once (e.g. to io.Discard) silences all of them.
// The mutex guards concurrent reads in Write against swaps in set.
type switchWriter struct {
	mu sync.RWMutex
	w  io.Writer
}

func (s *switchWriter) Write(p []byte) (int, error) {
	s.mu.RLock()
	w := s.w
	s.mu.RUnlock()
	return w.Write(p)
}

// set atomically replaces the target writer and returns the previous one.
func (s *switchWriter) set(w io.Writer) io.Writer {
	s.mu.Lock()
	prev := s.w
	s.w = w
	s.mu.Unlock()
	return prev
}

// SessionService orchestrates hive session operations.
type SessionService struct {
	sessions   session.Store
	git        git.Git
	config     *config.Config
	executor   executil.Executor
	log        zerolog.Logger
	bus        *eventbus.EventBus
	spawner    *Spawner
	recycler   *Recycler
	hookRunner *HookRunner
	fileCopier *FileCopier
	out        *switchWriter
	err        *switchWriter
	bareMu     sync.Map // map[remote → *sync.Mutex]
}

// NewSessionService creates a new SessionService.
func NewSessionService(
	sessions session.Store,
	gitClient git.Git,
	cfg *config.Config,
	bus *eventbus.EventBus,
	exec executil.Executor,
	renderer *tmpl.Renderer,
	log zerolog.Logger,
	stdout, stderr io.Writer,
) *SessionService {
	out := &switchWriter{w: stdout}
	err := &switchWriter{w: stderr}
	return &SessionService{
		sessions:   sessions,
		git:        gitClient,
		config:     cfg,
		bus:        bus,
		executor:   exec,
		log:        log,
		out:        out,
		err:        err,
		spawner:    NewSpawner(log.With().Str("component", "spawner").Logger(), exec, renderer, coretmux.New(exec, log.With().Str("component", "tmux").Logger()), out, err),
		recycler:   NewRecycler(log.With().Str("component", "recycler").Logger(), exec, renderer),
		hookRunner: NewHookRunner(log.With().Str("component", "hooks").Logger(), exec, out, err),
		fileCopier: NewFileCopier(log.With().Str("component", "copier").Logger(), out),
	}
}

// SilenceOutput redirects all output to io.Discard and returns a restore
// function that reverts to the previous writers. Call before starting the TUI
// to prevent hook and spawn output from corrupting the terminal display.
func (s *SessionService) SilenceOutput() (restore func()) {
	prevOut := s.out.set(io.Discard)
	prevErr := s.err.set(io.Discard)
	return func() {
		s.out.set(prevOut)
		s.err.set(prevErr)
	}
}

// CreateSession creates a new session or recycles an existing one.
func (s *SessionService) CreateSession(ctx context.Context, opts CreateOptions) (*session.Session, error) {
	s.log.Info().Str("name", opts.Name).Str("remote", opts.Remote).Msg("creating session")

	progress := opts.Progress

	// Redirect service output to progress writer when provided.
	if progress != nil {
		prevOut := s.out.set(progress)
		prevErr := s.err.set(progress)
		defer func() {
			s.out.set(prevOut)
			s.err.set(prevErr)
		}()
	}

	remote := opts.Remote
	if remote == "" {
		writeProgressf(progress, "Detecting remote...")
		var err error
		remote, err = s.DetectRemote(ctx, ".")
		if err != nil {
			return nil, fmt.Errorf("detect remote: %w", err)
		}
		s.log.Debug().Str("remote", remote).Msg("detected remote")
	}

	// Resolve clone strategy
	cloneStrategy := opts.CloneStrategy
	if cloneStrategy == "" {
		cloneStrategy = s.config.GetCloneStrategy(remote)
	}
	if err := config.ValidateCloneStrategy(cloneStrategy); err != nil {
		return nil, err
	}
	writeProgressf(progress, "Clone strategy: %s", cloneStrategy)

	var sess session.Session
	slug := session.Slugify(opts.Name)

	// Try to find and validate a recyclable session with matching strategy
	writeProgressf(progress, "Looking for recyclable session...")
	recyclable := s.findValidRecyclable(ctx, remote, cloneStrategy)

	if recyclable != nil {
		// Reuse existing recycled session (already cleaned up when marked for recycle)
		s.log.Debug().Str("session_id", recyclable.ID).Msg("found valid recyclable session")
		writeProgressf(progress, "Recycling session %s...", recyclable.ID)

		if cloneStrategy == config.CloneStrategyWorktree {
			// Worktrees are linked to a bare clone with no origin remote — use fetch+reset instead of pull.
			bareDir := s.bareDirForRemote(remote)
			writeProgressf(progress, "Fetching latest changes...")
			if err := s.git.Fetch(ctx, bareDir); err != nil {
				s.log.Warn().Err(err).Str("session_id", recyclable.ID).Msg("fetch failed, marking corrupted")
				s.markCorrupted(ctx, recyclable)
				recyclable = nil
			} else if err := s.git.WorktreeReset(ctx, bareDir, recyclable.Path); err != nil {
				s.log.Warn().Err(err).Str("session_id", recyclable.ID).Msg("worktree reset failed, marking corrupted")
				s.markCorrupted(ctx, recyclable)
				recyclable = nil
			}
		} else {
			// Pull latest changes before running hooks
			s.log.Debug().Str("path", recyclable.Path).Msg("pulling latest changes")
			writeProgressf(progress, "Pulling latest changes...")
			if err := s.git.Pull(ctx, recyclable.Path); err != nil {
				// Pull failed - mark as corrupted and fall through to clone
				s.log.Warn().Err(err).Str("session_id", recyclable.ID).Msg("pull failed, marking corrupted")
				s.markCorrupted(ctx, recyclable)
				recyclable = nil
			}
		}
	}

	if recyclable != nil {
		sess = *recyclable
		sess.Name = opts.Name
		sess.Slug = slug
		sess.State = session.StateActive
		sess.UpdatedAt = time.Now()
	} else {
		// Create new session (either no recyclable found or it was corrupted)
		sessID := opts.SessionID
		if sessID == "" {
			sessID = generateID()
		}
		dirID := generateID()
		repoName := git.ExtractRepoName(remote)

		var path string
		switch cloneStrategy {
		case config.CloneStrategyWorktree:
			path = filepath.Join(s.config.ReposDir(), fmt.Sprintf("%s-wt-%s", repoName, dirID))
		default:
			path = filepath.Join(s.config.ReposDir(), fmt.Sprintf("%s-%s", repoName, dirID))
		}

		s.log.Info().Str("remote", remote).Str("dest", path).Str("strategy", cloneStrategy).Msg("cloning repository")

		now := time.Now()
		sess = session.Session{
			ID:            sessID,
			Name:          opts.Name,
			Slug:          slug,
			Path:          path,
			Remote:        remote,
			State:         session.StateActive,
			CloneStrategy: cloneStrategy,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if cloneStrategy == config.CloneStrategyWorktree {
			bareDir, err := s.ensureBareClone(ctx, remote, progress)
			if err != nil {
				return nil, fmt.Errorf("ensure bare clone: %w", err)
			}
			writeProgressf(progress, "Adding worktree...")
			branch := "hive-" + dirID
			if err := s.git.WorktreeAdd(ctx, bareDir, path, branch); err != nil {
				return nil, fmt.Errorf("worktree add: %w", err)
			}
			sess.SetMeta(session.MetaWorktreeBranch, branch)
		} else {
			writeProgressf(progress, "Cloning repository...")
			if err := s.git.Clone(ctx, remote, path); err != nil {
				return nil, fmt.Errorf("clone repository: %w", err)
			}
		}

		writeProgressf(progress, "Clone complete")
		s.log.Debug().Msg("clone complete")
	}

	// Execute matching rules
	writeProgressf(progress, "Executing rules...")
	if err := s.executeRules(ctx, remote, opts.Source, sess.Path); err != nil {
		return nil, fmt.Errorf("execute rules: %w", err)
	}

	// Save session
	writeProgressf(progress, "Saving session...")
	if err := s.sessions.Save(ctx, sess); err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}

	// Spawn terminal
	writeProgressf(progress, "Spawning terminal...")
	owner, repoName := git.ExtractOwnerRepo(remote)
	data := SpawnData{
		Path:       sess.Path,
		Name:       sess.Name,
		Prompt:     opts.Prompt,
		Slug:       sess.Slug,
		ContextDir: s.config.RepoContextDir(owner, repoName),
		Owner:      owner,
		Repo:       repoName,
	}

	if !opts.SkipSpawn {
		strategy := config.ResolveSpawn(s.config.Rules, remote, opts.UseBatchSpawn)
		switch {
		case strategy.IsWindows():
			if err := s.spawner.SpawnWindows(ctx, strategy.Windows, data, opts.UseBatchSpawn || opts.Background); err != nil {
				return nil, fmt.Errorf("spawn terminal: %w", err)
			}
		case len(strategy.Commands) > 0:
			if err := s.spawner.Spawn(ctx, strategy.Commands, data); err != nil {
				return nil, fmt.Errorf("spawn terminal: %w", err)
			}
		default:
			return nil, fmt.Errorf("spawn terminal: no spawn strategy resolved for remote %q", remote)
		}
	}

	writeProgressf(progress, "Session created: %s", sess.Name)
	s.log.Info().Str("session_id", sess.ID).Str("path", sess.Path).Msg("session created")

	s.bus.PublishSessionCreated(eventbus.SessionCreatedPayload{Session: &sess})

	return &sess, nil
}

// writeProgressf writes a formatted progress line when w is non-nil.
func writeProgressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, format+"\n", args...)
}

// ListSessions returns all sessions.
func (s *SessionService) ListSessions(ctx context.Context) ([]session.Session, error) {
	return s.sessions.List(ctx)
}

// GetSession returns a session by ID.
func (s *SessionService) GetSession(ctx context.Context, id string) (session.Session, error) {
	return s.sessions.Get(ctx, id)
}

// RecycleSession marks a session for recycling and runs recycle commands.
// The session directory is not moved; only the DB record state changes.
// Output is written to w. If w is nil, output is discarded.
func (s *SessionService) RecycleSession(ctx context.Context, id string, w io.Writer) error {
	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	if !sess.CanRecycle() {
		return fmt.Errorf("session %s cannot be recycled (state: %s)", id, sess.State)
	}

	if sess.CloneStrategy == config.CloneStrategyWorktree {
		return s.recycleWorktreeSession(ctx, &sess)
	}

	// Full-clone recycle: validate, reset, and mark recycled.
	// Validate repository before recycling
	if err := s.git.IsValidRepo(ctx, sess.Path); err != nil {
		s.log.Warn().Err(err).Str("session_id", id).Msg("session has corrupted repository")
		s.markCorrupted(ctx, &sess)
		return fmt.Errorf("session %s has corrupted repository: %w", id, err)
	}

	// Get default branch for template
	defaultBranch, err := s.git.DefaultBranch(ctx, sess.Path)
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to get default branch, using 'main'")
		defaultBranch = "main"
	}

	data := RecycleData{
		DefaultBranch: defaultBranch,
	}

	if err := s.recycler.Recycle(ctx, sess.Path, s.config.GetRecycleCommands(sess.Remote), data, w); err != nil {
		return fmt.Errorf("recycle session %s: %w", id, err)
	}

	// Kill associated tmux session (best-effort)
	if _, err := s.executor.Run(ctx, "tmux", "kill-session", "-t", sess.Name); err != nil {
		s.log.Debug().Err(err).Str("session", sess.Name).Msg("no tmux session to kill")
	}

	sess.MarkRecycled(time.Now())

	if err := s.sessions.Save(ctx, sess); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	// Enforce max recycled limit
	if err := s.enforceMaxRecycled(ctx, sess.Remote, sess.CloneStrategy); err != nil {
		s.log.Warn().Err(err).Str("remote", sess.Remote).Msg("failed to enforce max recycled limit")
	}

	s.log.Info().Str("session_id", id).Str("path", sess.Path).Msg("session recycled")

	s.bus.PublishSessionRecycled(eventbus.SessionRecycledPayload{Session: &sess})

	return nil
}

// recycleWorktreeSession resets the worktree to origin's default branch and marks the session recycled.
// The worktree directory is kept on disk so it can be reused without re-cloning.
func (s *SessionService) recycleWorktreeSession(ctx context.Context, sess *session.Session) error {
	branch := sess.GetMeta(session.MetaWorktreeBranch)
	if branch == "" {
		s.log.Warn().Str("session_id", sess.ID).Msg("worktree session has no branch metadata, skipping git reset")
	} else {
		bareDir := s.bareDirForRemote(sess.Remote)
		if err := s.git.WorktreeReset(ctx, bareDir, sess.Path); err != nil {
			s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("worktree reset failed during recycle")
			// Continue — session is still marked recycled; findValidRecyclable will validate on reuse
		}
	}

	// Kill associated tmux session (best-effort)
	if _, err := s.executor.Run(ctx, "tmux", "kill-session", "-t", sess.Name); err != nil {
		s.log.Debug().Err(err).Str("session", sess.Name).Msg("no tmux session to kill")
	}

	sess.MarkRecycled(time.Now())

	if err := s.sessions.Save(ctx, *sess); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	// Enforce max recycled limit
	if err := s.enforceMaxRecycled(ctx, sess.Remote, sess.CloneStrategy); err != nil {
		s.log.Warn().Err(err).Str("remote", sess.Remote).Msg("failed to enforce max recycled limit")
	}

	s.log.Info().Str("session_id", sess.ID).Str("path", sess.Path).Msg("worktree session recycled")

	s.bus.PublishSessionRecycled(eventbus.SessionRecycledPayload{Session: sess})

	return nil
}

// RenameSession changes the name (and slug) of an existing session.
func (s *SessionService) RenameSession(ctx context.Context, id, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("rename session: name cannot be empty")
	}

	slug := session.Slugify(newName)
	if slug == "" {
		return fmt.Errorf("rename session: name %q produces an empty slug", newName)
	}

	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	oldName := sess.Name
	sess.Name = newName
	sess.Slug = slug
	sess.UpdatedAt = time.Now()

	if err := s.sessions.Save(ctx, sess); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	s.bus.PublishSessionRenamed(eventbus.SessionRenamedPayload{Session: &sess, OldName: oldName})

	s.log.Info().Str("session_id", id).Str("new_name", newName).Msg("session renamed")
	return nil
}

// SetSessionGroup sets or clears the user-assigned group for a session.
// An empty group clears the assignment.
func (s *SessionService) SetSessionGroup(ctx context.Context, id, group string) error {
	group = strings.TrimSpace(group)

	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	sess.SetGroup(group)
	sess.UpdatedAt = time.Now()

	if err := s.sessions.Save(ctx, sess); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	s.log.Info().Str("session_id", id).Str("group", group).Msg("session group updated")
	return nil
}

// DeleteSession removes a session and its directory.
func (s *SessionService) DeleteSession(ctx context.Context, id string) error {
	sess, err := s.sessions.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	s.log.Info().Str("session_id", id).Str("path", sess.Path).Msg("deleting session")

	// For worktree sessions, remove the worktree via git before os.RemoveAll so
	// the bare repo's internal worktree tracking stays consistent.
	if sess.CloneStrategy == config.CloneStrategyWorktree {
		branch := sess.GetMeta(session.MetaWorktreeBranch)
		if branch == "" {
			s.log.Warn().Str("session_id", id).Msg("worktree session has no branch metadata, skipping git cleanup")
		} else {
			bareDir := s.bareDirForRemote(sess.Remote)
			if err := s.git.WorktreeRemove(ctx, bareDir, sess.Path, branch); err != nil {
				s.log.Warn().Err(err).Str("session_id", id).Msg("worktree remove failed during delete, proceeding with RemoveAll")
			}
		}
	}

	// Remove directory
	if err := os.RemoveAll(sess.Path); err != nil {
		return fmt.Errorf("remove directory: %w", err)
	}

	// Delete from store
	if err := s.sessions.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	s.bus.PublishSessionDeleted(eventbus.SessionDeletedPayload{SessionID: id})

	return nil
}

// Prune removes recycled and corrupted sessions and their directories.
// If all is true, deletes ALL recycled sessions.
// If all is false, respects max_recycled limit per repository (keeps newest N).
func (s *SessionService) Prune(ctx context.Context, all bool) (int, error) {
	s.log.Info().Bool("all", all).Msg("pruning sessions")

	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list sessions: %w", err)
	}

	count := 0

	// Always delete corrupted sessions
	for _, sess := range sessions {
		if sess.State == session.StateCorrupted {
			if err := s.DeleteSession(ctx, sess.ID); err != nil {
				s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("failed to delete corrupted session")
				continue
			}
			count++
		}
	}

	if all {
		// Delete ALL recycled sessions
		for _, sess := range sessions {
			if sess.State == session.StateRecycled {
				if err := s.DeleteSession(ctx, sess.ID); err != nil {
					s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("failed to prune session")
					continue
				}
				count++
			}
		}
	} else {
		// Respect max_recycled limit per repository
		deleted, err := s.pruneExcessRecycled(ctx, sessions)
		if err != nil {
			return count, fmt.Errorf("prune excess recycled: %w", err)
		}
		count += deleted
	}

	s.log.Info().Int("count", count).Msg("prune complete")

	return count, nil
}

// pruneExcessRecycled deletes recycled sessions exceeding max_recycled per repository+strategy pool.
func (s *SessionService) pruneExcessRecycled(ctx context.Context, sessions []session.Session) (int, error) {
	type recyclePoolKey struct {
		remote   string
		strategy string
	}

	// Group recycled sessions by remote+strategy so full/worktree pools are independent.
	byPool := make(map[recyclePoolKey][]session.Session)
	for _, sess := range sessions {
		if sess.State == session.StateRecycled {
			strategy := sess.CloneStrategy
			if strategy == "" {
				strategy = config.CloneStrategyFull
			}

			key := recyclePoolKey{remote: sess.Remote, strategy: strategy}
			byPool[key] = append(byPool[key], sess)
		}
	}

	count := 0
	for key, recycled := range byPool {
		limit := s.config.GetMaxRecycled(key.remote)
		if limit == 0 || len(recycled) <= limit {
			continue
		}

		// Sort by UpdatedAt descending (newest first)
		sort.Slice(recycled, func(i, j int) bool {
			return recycled[i].UpdatedAt.After(recycled[j].UpdatedAt)
		})

		// Delete oldest sessions beyond the limit
		for _, sess := range recycled[limit:] {
			if err := s.DeleteSession(ctx, sess.ID); err != nil {
				s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("failed to delete excess session")
				continue
			}
			count++
		}
	}

	return count, nil
}

// DetectRemote gets the git remote URL from the specified directory.
func (s *SessionService) DetectRemote(ctx context.Context, dir string) (string, error) {
	return s.git.RemoteURL(ctx, dir)
}

// DetectSession returns the session ID for the current working directory.
// Returns empty string if not in a hive session.
func (s *SessionService) DetectSession(ctx context.Context) (string, error) {
	detector := messaging.NewSessionDetector(s.sessions)
	return detector.DetectSession(ctx)
}

// OpenTmuxSession opens (or creates) a tmux session for the given session parameters.
// It resolves the spawn strategy, renders window templates, and delegates to the spawner.
func (s *SessionService) OpenTmuxSession(ctx context.Context, name, path, remote, targetWindow string, background bool) error {
	strategy := config.ResolveSpawn(s.config.Rules, remote, false)
	if !strategy.IsWindows() {
		return fmt.Errorf("tmux action requires windows config (legacy spawn commands should use shell executor)")
	}

	owner, repo := git.ExtractOwnerRepo(remote)
	data := SpawnData{
		Path:       path,
		Name:       name,
		Slug:       session.Slugify(name),
		ContextDir: s.config.RepoContextDir(owner, repo),
		Owner:      owner,
		Repo:       repo,
	}

	return s.spawner.OpenWindows(ctx, strategy.Windows, data, background, targetWindow)
}

// Git returns the git client for use in background operations.
func (s *SessionService) Git() git.Git {
	return s.git
}

// generateID creates a 6-character random alphanumeric session ID.
func generateID() string {
	return randid.Generate(6)
}

// findValidRecyclable finds a recyclable session matching remote and cloneStrategy.
// Returns nil if none found or all candidates are corrupted.
func (s *SessionService) findValidRecyclable(ctx context.Context, remote, cloneStrategy string) *session.Session {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to list sessions")
		return nil
	}

	for i := range sessions {
		sess := &sessions[i]

		// Normalize empty strategy to "full" for comparison
		sessStrategy := sess.CloneStrategy
		if sessStrategy == "" {
			sessStrategy = config.CloneStrategyFull
		}

		// Skip non-recyclable sessions or strategy mismatch
		if sess.State != session.StateRecycled || sess.Remote != remote || sessStrategy != cloneStrategy {
			continue
		}

		// Validate the repository
		if err := s.git.IsValidRepo(ctx, sess.Path); err != nil {
			s.log.Warn().Err(err).Str("session_id", sess.ID).Str("path", sess.Path).Msg("corrupted session found")
			s.markCorrupted(ctx, sess)
			continue
		}

		return sess
	}

	return nil
}

// getBareCloneLock returns the per-remote mutex for bare clone creation.
func (s *SessionService) getBareCloneLock(remote string) *sync.Mutex {
	mu, _ := s.bareMu.LoadOrStore(remote, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// ensureBareClone returns the path to the bare clone of remote, creating or fetching it as needed.
// It serializes concurrent calls for the same remote to prevent duplicate clones.
func (s *SessionService) ensureBareClone(ctx context.Context, remote string, progress io.Writer) (string, error) {
	mu := s.getBareCloneLock(remote)
	mu.Lock()
	defer mu.Unlock()

	bareDir := s.bareDirForRemote(remote)
	_, statErr := os.Stat(bareDir)
	if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf("stat bare dir: %w", statErr)
	}
	if os.IsNotExist(statErr) {
		writeProgressf(progress, "Cloning bare repository (this may take a while)...")
		if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
			return "", fmt.Errorf("create bare parent: %w", err)
		}
		if err := s.git.CloneBare(ctx, remote, bareDir); err != nil {
			_ = os.RemoveAll(bareDir) // clean up partial clone
			return "", fmt.Errorf("bare clone: %w", err)
		}
	} else {
		writeProgressf(progress, "Fetching latest changes...")
		if err := s.git.Fetch(ctx, bareDir); err != nil {
			return "", fmt.Errorf("fetch bare: %w", err)
		}
	}
	return bareDir, nil
}

// bareDirForRemote returns the path where the bare clone for remote is stored.
func (s *SessionService) bareDirForRemote(remote string) string {
	owner, repo := git.ExtractOwnerRepo(remote)
	return filepath.Join(s.config.ReposDir(), ".bare", owner, repo)
}

// markCorrupted marks a session as corrupted and optionally deletes it.
func (s *SessionService) markCorrupted(ctx context.Context, sess *session.Session) {
	sess.MarkCorrupted(time.Now())
	s.bus.PublishSessionCorrupted(eventbus.SessionCorruptedPayload{Session: sess})

	if s.config.AutoDeleteCorrupted {
		s.log.Info().Str("session_id", sess.ID).Msg("auto-deleting corrupted session")
		if err := s.DeleteSession(ctx, sess.ID); err != nil {
			s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("failed to delete corrupted session, marking instead")
			// Fall through to save as corrupted
			if err := s.sessions.Save(ctx, *sess); err != nil {
				s.log.Error().Err(err).Str("session_id", sess.ID).Msg("failed to save corrupted session")
			}
		}
	} else {
		if err := s.sessions.Save(ctx, *sess); err != nil {
			s.log.Error().Err(err).Str("session_id", sess.ID).Msg("failed to save corrupted session")
		}
	}
}

// executeRules executes all rules matching the remote URL.
func (s *SessionService) executeRules(ctx context.Context, remote, source, dest string) error {
	for _, rule := range s.config.Rules {
		matched, err := matchRemotePattern(rule.Pattern, remote)
		if err != nil {
			return fmt.Errorf("match pattern %q: %w", rule.Pattern, err)
		}
		if !matched {
			continue
		}

		s.log.Debug().
			Str("pattern", rule.Pattern).
			Strs("commands", rule.Commands).
			Strs("copy", rule.Copy).
			Msg("rule matched")

		// Copy files first (so hooks can operate on them)
		if len(rule.Copy) > 0 && source != "" {
			if err := s.fileCopier.CopyFiles(ctx, rule, source, dest); err != nil {
				return fmt.Errorf("copy files: %w", err)
			}
		}

		// Run commands
		if len(rule.Commands) > 0 {
			if err := s.hookRunner.RunHooks(ctx, rule, dest); err != nil {
				return fmt.Errorf("run hooks: %w", err)
			}
		}
	}
	return nil
}

// enforceMaxRecycled deletes oldest recycled sessions for a remote+strategy when limit is exceeded.
func (s *SessionService) enforceMaxRecycled(ctx context.Context, remote, cloneStrategy string) error {
	limit := s.config.GetMaxRecycled(remote)
	if limit == 0 {
		// Unlimited
		return nil
	}

	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	// Normalize empty strategy to "full" for comparison
	if cloneStrategy == "" {
		cloneStrategy = config.CloneStrategyFull
	}

	// Collect recycled sessions for this remote+strategy
	var recycled []session.Session
	for _, sess := range sessions {
		sessStrategy := sess.CloneStrategy
		if sessStrategy == "" {
			sessStrategy = config.CloneStrategyFull
		}
		if sess.State == session.StateRecycled && sess.Remote == remote && sessStrategy == cloneStrategy {
			recycled = append(recycled, sess)
		}
	}

	// Nothing to enforce
	if len(recycled) <= limit {
		return nil
	}

	// Sort by UpdatedAt descending (newest first)
	sort.Slice(recycled, func(i, j int) bool {
		return recycled[i].UpdatedAt.After(recycled[j].UpdatedAt)
	})

	// Delete oldest sessions beyond the limit
	for _, sess := range recycled[limit:] {
		s.log.Info().
			Str("session_id", sess.ID).
			Str("remote", remote).
			Int("limit", limit).
			Msg("deleting excess recycled session")

		if err := s.DeleteSession(ctx, sess.ID); err != nil {
			s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("failed to delete excess recycled session")
		}
	}

	return nil
}

// AddWindowsToTmuxSession adds windows to an existing tmux session, converting action.WindowSpec
// to coretmux.RenderedWindow. Satisfies the command.WindowSpawner interface.
func (s *SessionService) AddWindowsToTmuxSession(ctx context.Context, tmuxName, workDir string, windows []action.WindowSpec, background bool) error {
	rendered := make([]coretmux.RenderedWindow, len(windows))
	for i, w := range windows {
		rendered[i] = coretmux.RenderedWindow{Name: w.Name, Command: w.Command, Dir: w.Dir, Focus: w.Focus}
	}
	return s.spawner.AddWindowsToTmuxSession(ctx, tmuxName, workDir, rendered, background)
}

// CreateSessionWithWindows creates a new Hive session, optionally runs shCmd in its directory,
// then opens tmux windows in it. Non-zero shCmd exit aborts window creation.
// Satisfies the command.WindowSpawner interface.
func (s *SessionService) CreateSessionWithWindows(ctx context.Context, req action.NewSessionRequest, windows []action.WindowSpec, background bool) error {
	sess, err := s.CreateSession(ctx, CreateOptions{
		Name:      req.Name,
		Remote:    req.Remote,
		SkipSpawn: true,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	cleanup := func() {
		if err := s.DeleteSession(ctx, sess.ID); err != nil {
			s.log.Warn().Err(err).Str("session_id", sess.ID).Msg("failed to clean up session after spawn failure")
		}
	}

	if req.ShCmd != "" {
		if err := executil.RunSh(ctx, sess.Path, req.ShCmd); err != nil {
			cleanup()
			return fmt.Errorf("sh: %w", err)
		}
	}

	rendered := make([]coretmux.RenderedWindow, len(windows))
	for i, w := range windows {
		rendered[i] = coretmux.RenderedWindow{Name: w.Name, Command: w.Command, Dir: w.Dir, Focus: w.Focus}
	}
	if err := s.spawner.tmux.CreateSession(ctx, sess.Name, sess.Path, rendered, background); err != nil {
		cleanup()
		return fmt.Errorf("create tmux session: %w", err)
	}
	return nil
}
