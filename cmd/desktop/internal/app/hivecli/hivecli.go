// Package hivecli installs the `hive` command as a symlink to this app's
// executable, which runs the CLI when invoked by that name, and reports what
// `hive` resolves to on the user's PATH.
//
// The link is its own ownership record: a symlink whose target is a
// hive-desktop executable is the app's, and anything else at the path belongs
// to someone else and is never written or removed.
package hivecli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CommandName is the name the command is installed under, and the argv[0]
// basename that makes the app executable run the CLI.
const CommandName = "hive"

const executableName = "hive-desktop"

// LinkPath is where the app installs the command.
func LinkPath(home string) string {
	return filepath.Join(home, ".local", "bin", CommandName)
}

// Link is what is at the install path.
type Link struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	AppOwned bool   `json:"appOwned"`
	// Target is the symlink's destination, or "" for a regular file.
	Target string `json:"target"`
}

// Inspect reports what is at path without following it.
func Inspect(path string) (Link, error) {
	link := Link{Path: path}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return link, nil
	}
	if err != nil {
		return link, err
	}
	link.Exists = true
	if info.Mode()&fs.ModeSymlink == 0 {
		return link, nil
	}
	target, err := os.Readlink(path)
	if err != nil {
		return link, err
	}
	link.Target = target
	link.AppOwned = filepath.Base(target) == executableName
	return link, nil
}

// Unsupported reports why executable cannot back the command, or "".
func Unsupported(executable, version string) string {
	switch {
	case version == "" || version == "dev":
		return "This is a development build. Only a release build installs the hive command."
	case strings.Contains(executable, "/AppTranslocation/"):
		return "macOS is running Hive from a temporary read-only location. Move Hive to Applications and reopen it."
	case filepath.Base(executable) != executableName:
		return fmt.Sprintf("Hive is running as %s, not its installed executable.", filepath.Base(executable))
	}
	return ""
}

// Action is what Sync did.
type Action string

const (
	ActionNone      Action = "none"
	ActionCreated   Action = "created"
	ActionRepointed Action = "repointed"
	ActionRemoved   Action = "removed"
	// ActionConflict means the command is wanted but another program's file
	// holds the path, so nothing was written.
	ActionConflict Action = "conflict"
)

// Sync makes the install path match enabled: a link to executable, or no link
// of the app's. It never touches a file the app did not create.
func Sync(path, executable string, enabled bool) (Action, error) {
	link, err := Inspect(path)
	if err != nil {
		return ActionNone, err
	}
	switch {
	case !link.Exists && !enabled:
		return ActionNone, nil
	case !link.Exists:
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return ActionNone, err
		}
		return ActionCreated, os.Symlink(executable, path)
	case !link.AppOwned && enabled:
		return ActionConflict, nil
	case !link.AppOwned:
		return ActionNone, nil
	case !enabled:
		return ActionRemoved, os.Remove(path)
	case link.Target == executable:
		return ActionNone, nil
	}
	return ActionRepointed, replaceLink(path, executable)
}

// replaceLink swaps the link in one rename, so a `hive` run during the swap
// finds either the old target or the new one.
func replaceLink(path, executable string) error {
	tmp := filepath.Join(filepath.Dir(path), "."+CommandName+".tmp")
	_ = os.Remove(tmp)
	if err := os.Symlink(executable, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Resolve returns the first `hive` on pathList that would run, or "".
func Resolve(pathList string) string {
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, CommandName)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

// OnPath reports whether dir is an entry of pathList.
func OnPath(pathList, dir string) bool {
	dir = filepath.Clean(dir)
	for _, entry := range filepath.SplitList(pathList) {
		if entry != "" && filepath.Clean(entry) == dir {
			return true
		}
	}
	return false
}

// ParseVersion reads the version out of `hive --version`, which prints
// "hive version <version> (<commit>) <date>". It is "" when out has no version.
func ParseVersion(out string) string {
	fields := strings.Fields(out)
	for i, field := range fields {
		if field == "version" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// SameVersion compares two versions with or without a leading "v".
func SameVersion(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}
