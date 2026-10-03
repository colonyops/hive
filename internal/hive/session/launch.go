package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/colonyops/hive/internal/domain/session"
)

// LaunchRequest asks for a detached session the way a launcher does: by name
// and by a repository that may be any spelling of a configured checkout's
// remote.
type LaunchRequest struct {
	Name            string
	Prompt          string
	Agent           string
	Repo            string
	CollisionSuffix string
	// Tags label the session for a reader inside hive. Empty and repeated
	// tags are dropped.
	Tags []string
}

// LaunchError is a failed CreateFromRequest, carrying what a caller needs to
// hand the request back as a retryable draft. Nothing should match on its
// text (ADR a-failed-session-creation-is-a-retryable-draft).
type LaunchError struct {
	Name   string
	Remote string
	// Destination and CloneStrategy come off CreateSessionError. They are
	// empty for a failure raised before a checkout was resolved, or after the
	// clone (a spawn that failed).
	Destination   string
	CloneStrategy string
	// LeftoverCheckout reports that Destination is still on disk. The
	// destination is named whether or not anything survives there: git removes
	// its own directory when it refuses a clone, and keeps a complete one when
	// a post-checkout hook fails. Only the second is a directory to delete.
	LeftoverCheckout bool
	// Step is the failed operation ("clone repository", "worktree add") when
	// CreateSessionError carries one, and otherwise the last progress line.
	Step   string
	Output string
	Err    error
}

func (e *LaunchError) Error() string {
	if e.Step == "" {
		return fmt.Sprintf("create hive session: %v", e.Err)
	}
	return fmt.Sprintf("create hive session: %v (at %q)", e.Err, e.Step)
}

func (e *LaunchError) Unwrap() error { return e.Err }

// CreateFromRequest creates a detached session for req. It prefers a
// configured local checkout whose remote is equivalent to req.Repo, so the
// session copies files from it.
//
// A duplicate name returns session.ErrDuplicateName unwrapped, because the
// caller asks for another name rather than showing a failure. Any other
// creation failure is a *LaunchError.
func (s *Service) CreateFromRequest(ctx context.Context, req LaunchRequest) (session.Session, error) {
	repo, err := s.ResolveSessionLaunchRepository(ctx, req.Repo)
	if err != nil {
		return session.Session{}, fmt.Errorf("resolve launch repository: %w", err)
	}

	// One progress log per attempt: CreateSessionError names the operation
	// that failed but not the steps before it, so without this a clone
	// failure arrives as "clone repository: git clone: exec git: exit status
	// 1" and nothing else.
	progress := &progressLog{}
	sess, err := s.CreateSession(ctx, CreateOptions{
		Name:            req.Name,
		Prompt:          req.Prompt,
		Remote:          repo.Remote,
		Source:          repo.Source,
		AgentKey:        req.Agent,
		Background:      true,
		CollisionSuffix: req.CollisionSuffix,
		Tags:            uniqueTags(req.Tags),
		Progress:        progress,
	})
	if err != nil {
		if errors.Is(err, session.ErrDuplicateName) {
			return session.Session{}, err
		}
		failure := &LaunchError{
			Name:   req.Name,
			Remote: repo.Remote,
			Step:   progress.LastLine(),
			Output: progress.Tail(),
			Err:    err,
		}
		// The typed error's Operation beats the last progress line: it is the
		// authority on which step failed, not a guess at what printed last.
		if created, ok := errors.AsType[*CreateSessionError](err); ok {
			failure.Destination, failure.CloneStrategy = created.Destination, created.CloneStrategy
			failure.LeftoverCheckout = isDir(created.Destination)
			if created.Operation != "" {
				failure.Step = created.Operation
			}
		}
		return session.Session{}, failure
	}
	return *sess, nil
}

func uniqueTags(tags []string) []string {
	var unique []string
	for _, tag := range tags {
		if tag != "" && !slices.Contains(unique, tag) {
			unique = append(unique, tag)
		}
	}
	return unique
}

func isDir(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// maxProgressTail bounds what travels with a failure: enough for the step
// lines and a hook's last words, not a build log.
const maxProgressTail = 2 << 10

// maxProgressTailLines bounds it from the other end, so a chatty hook cannot
// push every step line out of view.
const maxProgressTailLines = 20

// progressLog collects CreateOptions.Progress for one attempt: the step
// lines, plus whatever the session's hooks and file copies print, because
// CreateSession redirects the service writers at the same target.
//
// CreateSession swaps those writers service-wide for the call, so two
// concurrent creates interleave. That costs a misattributed diagnostic line.
type progressLog struct {
	mu    sync.Mutex
	lines []string
	// A step line arrives whole, but a hook's output arrives in whatever
	// chunks the pipe delivers, and a chunk is not a line.
	partial string
}

func (p *progressLog) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	text := p.partial + string(b)
	for {
		end := strings.IndexByte(text, '\n')
		if end < 0 {
			break
		}
		p.append(text[:end])
		text = text[end+1:]
	}
	if len(text) > maxProgressTail {
		text = text[len(text)-maxProgressTail:]
	}
	p.partial = text
	return len(b), nil
}

// Capped at what Tail can show: creation runs as long as a clone and a hook
// set take, so an unbounded buffer is a leak with a chatty hook in front of it.
func (p *progressLog) append(line string) {
	line = strings.TrimRight(line, "\r \t")
	if line == "" {
		return
	}
	p.lines = append(p.lines, line)
	if len(p.lines) > maxProgressTailLines {
		p.lines = p.lines[len(p.lines)-maxProgressTailLines:]
	}
}

// LastLine is the step the attempt died on, derived rather than matched
// against the step wording, which is free to change.
func (p *progressLog) LastLine() string {
	lines := p.snapshot()
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

func (p *progressLog) Tail() string {
	tail := strings.Join(p.snapshot(), "\n")
	if len(tail) > maxProgressTail {
		tail = strings.ToValidUTF8(tail[len(tail)-maxProgressTail:], "")
	}
	return tail
}

// The unterminated line counts: for a hook that died on a prompt it is the
// only line there is.
func (p *progressLog) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	lines := slices.Clone(p.lines)
	if partial := strings.TrimRight(p.partial, "\r \t"); partial != "" {
		lines = append(lines, partial)
	}
	return lines
}
