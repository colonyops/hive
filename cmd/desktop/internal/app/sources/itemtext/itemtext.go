// Package itemtext derives the session text an inbox item suggests: the branch
// name a session on it would use, and the default prompt the launch dialog
// opens with. The wording mirrors the hive TUI's source templates
// (SourceTemplateConfig defaults), so an item acted on from the desktop lands
// on the same branch name it would have in the TUI.
//
// A leaf, for the same reason connector and canonical are: every forge
// connector shapes items this way and they must not import each other to share
// it.
package itemtext

import (
	"fmt"
	"strings"
)

// Kinds an item can take. They are the payload's `kind` values, so the
// suggestion functions switch on the same strings a downstream node routes on.
const (
	KindPR    = "PR"
	KindIssue = "Issue"
)

// slugMaxLen caps the title portion of a branch name, so a long title cannot
// produce a branch nobody can type.
const slugMaxLen = 40

// Branch proposes a session branch name for acting on an item:
// "<prefix>-pr-<num>-<slug>" for a pull request, "<prefix>-<num>-<slug>" for an
// issue. prefix identifies the forge ("gh", "gitea").
func Branch(prefix, kind string, num int, title string) string {
	if kind == KindPR {
		prefix += "-pr"
	}
	return fmt.Sprintf("%s-%d-%s", prefix, num, Slug(title))
}

// Prompt proposes the launch dialog's default prompt: pull requests get
// "Review pull request", issues get "Work on" followed by the body. The result
// is trimmed, so an empty body leaves no trailing blank lines.
func Prompt(kind, title, url, body string) string {
	if kind == KindPR {
		return strings.TrimSpace(fmt.Sprintf("Review pull request %s\n\n%s", title, url))
	}
	return strings.TrimSpace(fmt.Sprintf("Work on %s\n\n%s\n\n%s", title, url, body))
}

// Slug renders a title as a git-branch-safe kebab-case fragment, capped at
// slugMaxLen.
func Slug(s string) string {
	var b strings.Builder
	lastDash := true // suppress leading dash
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= slugMaxLen {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
