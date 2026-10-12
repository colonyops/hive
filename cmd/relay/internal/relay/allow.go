package relay

import (
	"path"
	"sync/atomic"
)

// Allowlist admits an email that matches one of its glob patterns or is a
// listed user. Reload swaps the whole snapshot, so a check never sees half
// of an old list and half of a new one.
type Allowlist struct {
	snap atomic.Pointer[allowSnapshot]
}

type allowSnapshot struct {
	patterns []string
	users    map[string]struct{}
}

// NewAllowlist builds an Allowlist. Patterns must already compile with
// path.Match; config validates them. An empty list with no users admits
// nobody.
func NewAllowlist(patterns []string, users []string) *Allowlist {
	a := &Allowlist{}
	a.Reload(patterns, users)
	return a
}

func (a *Allowlist) Reload(patterns []string, users []string) {
	snap := &allowSnapshot{users: make(map[string]struct{}, len(users))}
	for _, p := range patterns {
		if p = NormalizeEmail(p); p != "" {
			snap.patterns = append(snap.patterns, p)
		}
	}
	for _, u := range users {
		if u = NormalizeEmail(u); u != "" {
			snap.users[u] = struct{}{}
		}
	}
	a.snap.Store(snap)
}

func (a *Allowlist) Allow(email string) bool {
	email = NormalizeEmail(email)
	if email == "" {
		return false
	}
	snap := a.snap.Load()
	if _, ok := snap.users[email]; ok {
		return true
	}
	for _, p := range snap.patterns {
		if ok, _ := path.Match(p, email); ok {
			return true
		}
	}
	return false
}

// HasPatterns reports whether any glob pattern is set. Listed users alone do
// not enable email codes: they sign in with a password.
func (a *Allowlist) HasPatterns() bool {
	return len(a.snap.Load().patterns) > 0
}
