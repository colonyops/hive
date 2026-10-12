// Package drive models Hive Drive: one document tree under the context root,
// split into spaces (a repository, a Chats workspace, or the shared space)
// that hold folders and markdown, html, and canvas documents.
package drive

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SpaceKind is what a space belongs to.
//
// ENUM(repository, workspace, shared)
type SpaceKind string

// Space is one root in the drive. Key is the space root's path under the
// drive root and is the identity used on disk, in hive:// links, and in every
// binding; Name is what a listing shows.
type Space struct {
	Kind SpaceKind `json:"kind"`
	Key  string    `json:"key"`  // "owner/repo", "workspaces/<name>", "shared"
	Name string    `json:"name"` // "owner/repo", "<name>", "Shared"
}

// WorkspacesDir and SharedKey are reserved top-level names under the drive
// root, so no repository owner may use them.
const (
	WorkspacesDir = "workspaces"
	SharedKey     = "shared"
)

// Shared is the space every agent outside a session or a chat writes to.
var Shared = Space{Kind: SpaceKindShared, Key: SharedKey, Name: "Shared"}

// RepositorySpace is the space of one repository. Each part must be a single
// local path component; case is kept because a GitHub owner is case-sensitive
// in links even though the file system may fold it.
func RepositorySpace(owner, repo string) (Space, error) {
	if !isPathComponent(owner) || !isPathComponent(repo) || isReserved(owner) {
		return Space{}, fmt.Errorf("%w: %q", ErrInvalidSpace, owner+"/"+repo)
	}
	key := owner + "/" + repo
	return Space{Kind: SpaceKindRepository, Key: key, Name: key}, nil
}

// WorkspaceSpace is the space of one Chats workspace, named by its directory.
func WorkspaceSpace(name string) (Space, error) {
	if !isPathComponent(name) {
		return Space{}, fmt.Errorf("%w: %q", ErrInvalidSpace, WorkspacesDir+"/"+name)
	}
	return Space{Kind: SpaceKindWorkspace, Key: WorkspacesDir + "/" + name, Name: name}, nil
}

// ParseSpaceKey is the inverse of Shared, RepositorySpace, and WorkspaceSpace.
func ParseSpaceKey(key string) (Space, error) {
	if key == SharedKey {
		return Shared, nil
	}
	first, rest, found := strings.Cut(key, "/")
	if !found {
		return Space{}, fmt.Errorf("%w: %q", ErrInvalidSpace, key)
	}
	if first == WorkspacesDir {
		return WorkspaceSpace(rest)
	}
	return RepositorySpace(first, rest)
}

// isReserved folds case because the drive root may sit on a case-insensitive
// file system, where "Shared" and "shared" are one directory.
func isReserved(owner string) bool {
	return strings.EqualFold(owner, WorkspacesDir) || strings.EqualFold(owner, SharedKey)
}

// isPathComponent reports whether s names exactly one directory entry: not
// empty, not a dot entry, no separator, and nothing filepath would refuse on
// this platform.
func isPathComponent(s string) bool {
	if s == "" || s == "." {
		return false
	}
	return filepath.Base(s) == s && filepath.IsLocal(s)
}
