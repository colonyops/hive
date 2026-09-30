package dispatch

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
)

// leftoverCheckout reports whether a failed attempt's destination survived it.
func leftoverCheckout(destination string) bool {
	if destination == "" {
		return false
	}
	info, err := os.Stat(destination)
	return err == nil && info.IsDir()
}

// maxProgressTail bounds what travels with the failure: enough for the step
// lines and a hook's last words, not a build log.
const maxProgressTail = 2 << 10

// maxProgressTailLines bounds it from the other end, so a chatty hook cannot
// push every step line out of view.
const maxProgressTailLines = 20

// sessionProgress collects hive.CreateOptions.Progress for one attempt: hive's
// step lines, plus whatever the session's hooks and file copies print, because
// hive redirects its service writers at the same target.
//
// hive swaps those writers service-wide for the call, so two concurrent creates
// interleave. That is a vendored flaw costing a misattributed diagnostic line.
type sessionProgress struct {
	mu    sync.Mutex
	lines []string
	// hive writes one whole step line per call, but a hook's output arrives in
	// whatever chunks the pipe delivers, and a chunk is not a line.
	partial string
}

func (p *sessionProgress) Write(b []byte) (int, error) {
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

// Capped at what Tail can show: creation runs as long as a clone and a hook set
// take, so an unbounded buffer is a leak with a chatty hook in front of it.
func (p *sessionProgress) append(line string) {
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
// against hive's phrasing, which hive is free to reword.
func (p *sessionProgress) LastLine() string {
	lines := p.snapshot()
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

func (p *sessionProgress) Tail() string {
	tail := strings.Join(p.snapshot(), "\n")
	if len(tail) > maxProgressTail {
		tail = strings.ToValidUTF8(tail[len(tail)-maxProgressTail:], "")
	}
	return tail
}

// The unterminated line counts: for a hook that died on a prompt it is the only
// line there is.
func (p *sessionProgress) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	lines := slices.Clone(p.lines)
	if partial := strings.TrimRight(p.partial, "\r \t"); partial != "" {
		lines = append(lines, partial)
	}
	return lines
}

// SessionCreateError is what the app layer reads to log a failed creation and
// hand the form back. Nothing matches on its text (Typed errors, mapped once
// per adapter, architecture.md; ADR a-failed-session-creation-is-a-retryable-draft).
type SessionCreateError struct {
	Name   string
	Remote string
	// Destination and CloneStrategy come off hive's own CreateSessionError.
	// They are empty for a failure raised before hive resolved a checkout --
	// a duplicate name, an invalid name, a spawn that failed after the clone.
	Destination   string
	CloneStrategy string
	// LeftoverCheckout reports that Destination is still on disk. hive names
	// the destination whether or not anything survives there: git removes its
	// own directory when it refuses a clone, and keeps a complete one when a
	// post-checkout hook fails. Only the second is a directory to go delete.
	LeftoverCheckout bool
	// Step is hive's failed operation ("clone repository", "worktree add") when
	// its typed error carries one, and otherwise the last progress line.
	Step   string
	Output string
	Err    error
}

func (e *SessionCreateError) Error() string {
	if e.Step == "" {
		return fmt.Sprintf("create hive session: %v", e.Err)
	}
	return fmt.Sprintf("create hive session: %v (at %q)", e.Err, e.Step)
}

func (e *SessionCreateError) Unwrap() error { return e.Err }
