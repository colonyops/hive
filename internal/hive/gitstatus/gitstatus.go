// Package gitstatus reads the git state of session checkouts: branch, dirty
// tree, unpushed commits and the line delta against the default branch.
package gitstatus

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/colonyops/hive/internal/domain/session"
	"github.com/colonyops/hive/internal/platform/git"
)

const readTimeout = 5 * time.Second

// Git is the subset of git.Git a status read needs.
type Git interface {
	Branch(ctx context.Context, dir string) (string, error)
	IsClean(ctx context.Context, dir string) (bool, error)
	HasUnpushedCommits(ctx context.Context, dir string) (bool, error)
	DiffStats(ctx context.Context, dir string) (additions, deletions int, err error)
}

// Status is one checkout's git state.
//
// Resolved separates "git answered" from the zero value. Err carries why a
// read failed and is never a substitute for it: a failed IsClean leaves Dirty
// false rather than assuming dirty, because a badge claiming uncommitted work
// on a git that never ran is a lie the user acts on.
type Status struct {
	Path     string
	Branch   string
	Dirty    bool
	Unpushed bool
	// Additions and Deletions are lines against the default branch, not HEAD.
	Additions int
	Deletions int
	// Host, Owner and Repo are the remote's coordinates. They are empty for a
	// remote that names no host, such as a local path.
	Host     string
	Owner    string
	Repo     string
	Resolved bool
	// Err is the first failed read of the branch, the dirty check or the diff.
	Err error
	// UnpushedErr is kept apart from Err because a branch with no upstream
	// fails this read on every poll, and that must not hide the rest.
	UnpushedErr error
}

// Error returns the failure a caller shows, preferring a checkout read over
// the routine unpushed failure.
func (s Status) Error() error {
	if s.Err != nil {
		return s.Err
	}
	return s.UnpushedErr
}

// Options picks the optional reads. The unpushed check costs up to three git
// calls, so a caller that does not show it leaves it off.
type Options struct {
	Unpushed bool
}

type Service struct {
	git     Git
	workers int
}

// NewService returns a Service that runs at most workers reads at once in a
// batch. A workers value below one means one.
func NewService(g Git, workers int) *Service {
	return &Service{git: g, workers: max(workers, 1)}
}

// Read reads one checkout. A failed branch read stands for the whole status,
// because every other read needs the working checkout it proves.
func (s *Service) Read(ctx context.Context, path string, opts Options) Status {
	status := Status{Path: path}

	branch, err := s.git.Branch(ctx, path)
	if err != nil {
		log.Debug().Err(err).Str("path", path).Msg("git branch lookup failed")
		status.Err = err
		return status
	}
	status.Branch = branch
	status.Resolved = true

	if clean, err := s.git.IsClean(ctx, path); err == nil {
		status.Dirty = !clean
	} else {
		log.Debug().Err(err).Str("path", path).Msg("git clean check failed")
		status.Err = err
	}
	if opts.Unpushed {
		if unpushed, err := s.git.HasUnpushedCommits(ctx, path); err == nil {
			status.Unpushed = unpushed
		} else {
			status.UnpushedErr = err
		}
	}
	if additions, deletions, err := s.git.DiffStats(ctx, path); err == nil {
		status.Additions, status.Deletions = additions, deletions
	} else {
		log.Debug().Err(err).Str("path", path).Msg("git diff stats lookup failed")
		if status.Err == nil {
			status.Err = err
		}
	}
	return status
}

// ReadSession reads a session's checkout with every read, and adds the
// remote's coordinates. A session with no checkout (not active, or no path)
// returns the zero Status: there is nothing to read, which is not a failure.
func (s *Service) ReadSession(ctx context.Context, sess session.Session) Status {
	if sess.State != session.StateActive || sess.Path == "" {
		return Status{}
	}
	status := s.Read(ctx, sess.Path, Options{Unpushed: true})
	status.Host, status.Owner, status.Repo = RemoteCoordinates(sess.Remote)
	return status
}

// ReadBatch reads every path, each under its own timeout, with at most the
// service's worker count running at once.
func (s *Service) ReadBatch(ctx context.Context, paths []string, opts Options) map[string]Status {
	results := make(map[string]Status, len(paths))
	var mu sync.Mutex
	sem := make(chan struct{}, s.workers)
	var wg sync.WaitGroup
	for _, path := range paths {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()

			readCtx, cancel := context.WithTimeout(ctx, readTimeout)
			defer cancel()
			status := s.Read(readCtx, path, opts)

			mu.Lock()
			results[path] = status
			mu.Unlock()
		})
	}
	wg.Wait()
	return results
}

// RemoteCoordinates reads the host, owner and repo off a remote. The host is
// lowered so a forge match and a cache key are canonical however the remote
// spells it.
//
// A remote naming no host returns nothing: git.ExtractOwnerRepo is
// host-agnostic and would read the last two path segments of a local path.
func RemoteCoordinates(remote string) (host, owner, repo string) {
	host = strings.ToLower(git.ExtractHost(remote))
	if host == "" {
		return "", "", ""
	}
	owner, repo = git.ExtractOwnerRepo(remote)
	return host, owner, repo
}
