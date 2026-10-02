// Package releasenotes reads the changelog a program ships inside its binary:
// what changed in each release, what a build carries that no release has yet,
// and whether the user has seen either.
//
// Each program owns its changelog and embeds it; this package parses one
// handed to it as an fs.FS and embeds nothing (ADR
// release-notes-ship-inside-the-binary).
//
// A changelog holds one `<version>.md` entry per release. Everything else
// accumulates in unreleased/ as one file per change, which a build embeds and
// renders as the draft, and which promotion collapses into the next entry (ADR
// release-notes-accumulate-as-fragments).
package releasenotes

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Entry is one set of release notes.
type Entry struct {
	// Version is the published version, without a v or desktop-v prefix. It
	// is empty on the draft.
	Version string
	// Date is the release date, zero on the draft.
	Date time.Time
	// Summary is the one line that heads a release's notes wherever they are
	// shown: the desktop's What's New toast and update manifest, and a
	// program's section of the GitHub release. The draft has none: a summary
	// describes a whole release, so it is written at promotion.
	Summary string
	// Body is the markdown detail, which may be empty for a release whose
	// summary says everything.
	Body string
	// Draft marks the unreleased entry. At most one exists, it is dropped
	// entirely when no fragment has landed, and it sorts above every release.
	Draft bool
}

// Entries is a set of release notes ordered newest first.
type Entries []Entry

// Load parses the changelog at the root of fsys. An error means a malformed
// entry or fragment was committed, which each program's changelog test and
// the release gate both catch before it can ship.
func Load(fsys fs.FS) (Entries, error) {
	files, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read changelog: %w", err)
	}
	entries := make(Entries, 0, len(files))
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read changelog %s: %w", name, err)
		}
		entry, err := parseEntry(name, raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	fragments, err := Fragments(fsys)
	if err != nil {
		return nil, err
	}
	// An empty draft is the state a promotion leaves behind, and it lasts
	// until the next change lands. Dropping it here is what keeps every
	// reader from having to ask whether the draft says anything.
	if len(fragments) > 0 {
		entries = append(entries, Entry{Draft: true, Body: renderDraft(fragments)})
	}

	slices.SortFunc(entries, func(a, b Entry) int { return -compareEntries(a, b) })
	return entries, nil
}

// Fragments returns the unreleased changes in the changelog at the root of
// fsys, unordered.
func Fragments(fsys fs.FS) ([]Fragment, error) {
	files, err := fs.ReadDir(fsys, UnreleasedDir)
	if errors.Is(err, fs.ErrNotExist) {
		// git does not track an empty directory, so a worktree whose .gitkeep
		// is gone has no unreleased/ between a promotion and the next change.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read changelog %s: %w", UnreleasedDir, err)
	}
	fragments := make([]Fragment, 0, len(files))
	for _, file := range files {
		name := file.Name()
		if file.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(UnreleasedDir, name))
		if err != nil {
			return nil, fmt.Errorf("read changelog %s/%s: %w", UnreleasedDir, name, err)
		}
		fragment, err := parseFragment(name, raw)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, fragment)
	}
	return fragments, nil
}

func compareEntries(a, b Entry) int {
	if a.Draft != b.Draft {
		if a.Draft {
			return 1
		}
		return -1
	}
	left, _ := parseVersion(a.Version)
	right, _ := parseVersion(b.Version)
	return compare(left, right)
}

// frontmatter is the YAML header a released changelog entry carries.
type frontmatter struct {
	Version string `yaml:"version"`
	Date    string `yaml:"date"`
	Summary string `yaml:"summary"`
}

// parseEntry reads one `<version>.md`. The filename is checked against the
// version in the header so a copy-pasted entry cannot silently describe the
// wrong release.
func parseEntry(filename string, raw []byte) (Entry, error) {
	// next.md was the draft until it became a directory. A branch that
	// predates the change still edits it, and its merge is clean, so say what
	// happened rather than failing as a malformed version.
	if filename == "next.md" {
		return Entry{}, fmt.Errorf(
			"changelog next.md: the draft is now one file per change in %s/; move each bullet with `mise run changelog:new` and delete this file (ADR release-notes-accumulate-as-fragments)",
			UnreleasedDir)
	}

	header, body, err := splitFrontmatter(raw)
	if err != nil {
		return Entry{}, fmt.Errorf("changelog %s: %w", filename, err)
	}
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		return Entry{}, fmt.Errorf("changelog %s: parse frontmatter: %w", filename, err)
	}

	stem := strings.TrimSuffix(filename, ".md")
	if fm.Version != stem {
		return Entry{}, fmt.Errorf("changelog %s: version %q does not match its filename", filename, fm.Version)
	}
	parsed, ok := parseVersion(fm.Version)
	if !ok {
		return Entry{}, fmt.Errorf("changelog %s: %q is not a publishable version", filename, fm.Version)
	}
	if parsed.prerelease != "" {
		return Entry{}, fmt.Errorf(
			"changelog %s: %q is a prerelease, and only a stable release gets an entry of its own; unreleased work belongs in %s/",
			filename, fm.Version, UnreleasedDir)
	}
	date, err := time.Parse(time.DateOnly, fm.Date)
	if err != nil {
		return Entry{}, fmt.Errorf("changelog %s: date must be YYYY-MM-DD: %w", filename, err)
	}

	return Entry{
		Version: fm.Version,
		Date:    date,
		Summary: strings.TrimSpace(fm.Summary),
		Body:    strings.TrimSpace(body),
	}, nil
}

func splitFrontmatter(raw []byte) (header, body string, err error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", fmt.Errorf("missing --- frontmatter header")
	}
	header, after, found := strings.Cut(text[len("---\n"):], "\n---")
	if !found {
		return "", "", fmt.Errorf("unterminated frontmatter header")
	}
	return header, strings.TrimPrefix(after, "\n"), nil
}

// Between returns what a launch on current should be shown, given after as the
// newest version whose notes have already been seen: the stable releases
// crossed, plus the draft, which describes work this build carries that no
// stable release does.
//
// An unparseable after — no recorded version — means no lower bound; an
// unparseable current means nothing to show, because a build outside the
// publishable set has no release to describe.
func (e Entries) Between(after, current string) Entries {
	to, ok := parseVersion(current)
	if !ok {
		return nil
	}
	from, hasFrom := parseVersion(after)

	out := make(Entries, 0, len(e))
	for _, entry := range e {
		if entry.Draft {
			out = append(out, entry)
			continue
		}
		v, ok := parseVersion(entry.Version)
		if !ok || compare(v, to) > 0 {
			continue
		}
		if hasFrom && compare(v, from) <= 0 {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// Find returns the entry describing version. The draft is never a match: it
// describes no version until it is promoted into one.
func (e Entries) Find(version string) (Entry, bool) {
	target, ok := parseVersion(version)
	if !ok {
		return Entry{}, false
	}
	for _, entry := range e {
		if entry.Draft {
			continue
		}
		if v, ok := parseVersion(entry.Version); ok && compare(v, target) == 0 {
			return entry, true
		}
	}
	return Entry{}, false
}

// Draft returns the accumulating entry for unreleased work. ok is false when
// nothing has landed since the last stable release.
func (e Entries) Draft() (Entry, bool) {
	for _, entry := range e {
		if entry.Draft {
			return entry, true
		}
	}
	return Entry{}, false
}
