package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/releasenotes"
)

// releaseEntry returns p's entry for version, read through p's embed package:
// the tree this tool was compiled from, which at the release commit is the
// tree the binaries are built from.
//
// Every release has an entry per program, promoted from the drafts before the
// release commit.
func releaseEntry(p product, version releaseVersion) (releasenotes.Entry, error) {
	entries, err := p.embedded()
	if err != nil {
		return releasenotes.Entry{}, err
	}
	entry, ok := entries.Find(version.String())
	if !ok {
		return releasenotes.Entry{}, fmt.Errorf(
			"no %s changelog entry for %s: promote the drafts with `mise run changelog:promote` and land them before releasing",
			p.name, version)
	}
	if entry.Summary == "" {
		return releasenotes.Entry{}, fmt.Errorf("%s changelog entry for %s has no summary", p.name, version)
	}
	return entry, nil
}

// notesFor returns the desktop notes the update manifests carry.
func notesFor(version releaseVersion) (releasenotes.Entry, error) {
	return releaseEntry(desktopProduct, version)
}

// validateChangelogEntry is the release gate. It runs with the other fail-fast
// checks, before anything is built: the desktop embeds its notes, and the
// publish workflow renders the CLI's from the tagged commit, so an entry
// written after the build would describe a release that cannot show it.
func validateChangelogEntry(version releaseVersion) error {
	return validateEntries(products, version)
}

func validateEntries(products []product, version releaseVersion) error {
	for _, p := range products {
		if _, err := releaseEntry(p, version); err != nil {
			return err
		}
	}
	return nil
}

// releaseNotesBody assembles the GitHub release body for version from every
// program's embedded entry.
func releaseNotesBody(version releaseVersion, downloadBase string) (string, error) {
	notes := make([]productNotes, 0, len(products))
	for _, p := range products {
		entry, err := releaseEntry(p, version)
		if err != nil {
			return "", err
		}
		notes = append(notes, productNotes{product: p, entry: entry})
	}
	return renderReleaseNotesBody(version, downloadBase, notes), nil
}

type productNotes struct {
	product product
	entry   releasenotes.Entry
}

// renderReleaseNotesBody writes the R2 download header, then each program's
// notes under its own heading.
func renderReleaseNotesBody(version releaseVersion, downloadBase string, notes []productNotes) string {
	var b strings.Builder
	b.WriteString(releaseNotesHeader(version, downloadBase))
	for _, n := range notes {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", n.product.title, n.entry.Summary)
		if n.entry.Body != "" {
			fmt.Fprintf(&b, "\n%s\n", demoteHeadings(n.entry.Body))
		}
	}
	return b.String()
}

// demoteHeadings moves an entry's sections one level down, under the
// program's heading. A line inside a fenced code block is left alone.
func demoteHeadings(body string) string {
	lines := strings.Split(body, "\n")
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if !fenced && strings.HasPrefix(line, "#") {
			lines[i] = "#" + line
		}
	}
	return strings.Join(lines, "\n")
}

// promoteTargetVersion resolves the version promote writes. With no explicit
// version it bumps the newest one from the same sources planRelease uses,
// tags and the live manifests, because the R2 history predates this
// repository.
//
// An entry that is promoted but not yet released counts as well. The release
// publishes the newest pending version, so an entry below it would never be
// tagged, yet the binaries would show it as a release that happened.
func promoteTargetVersion(ctx context.Context, explicit string, level bumpLevel) (releaseVersion, error) {
	published, _, err := releaseVersions(ctx)
	if err != nil {
		return releaseVersion{}, err
	}
	promoted, err := promotedVersions(products)
	if err != nil {
		return releaseVersion{}, err
	}
	taken := slices.Concat(published, promoted)
	if explicit == "" {
		return nextVersion(taken, level), nil
	}
	version, err := parsePublishVersion(explicit)
	if err != nil {
		return releaseVersion{}, fmt.Errorf("invalid version %q: %w", explicit, err)
	}
	if err := requireAdvances(version, taken, "published or promoted"); err != nil {
		return releaseVersion{}, err
	}
	return version, nil
}

// promotedVersions reads every entry version in the worktree's changelogs.
func promotedVersions(products []product) ([]releaseVersion, error) {
	var versions []releaseVersion
	for _, p := range products {
		entries, err := p.load(p.onDisk())
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Draft {
				continue
			}
			version, err := parsePublishVersion(entry.Version)
			if err != nil {
				return nil, fmt.Errorf("%s changelog entry %s: %w", p.name, entry.Version, err)
			}
			versions = append(versions, version)
		}
	}
	return versions, nil
}

// requireAdvances refuses a version that is not newer than every version in
// taken. what names what taken holds, for the error.
func requireAdvances(version releaseVersion, taken []releaseVersion, what string) error {
	if newest, ok := newestVersion(taken); ok && compareVersions(version, newest) <= 0 {
		return fmt.Errorf("%s does not advance the newest %s version %s", version, what, newest)
	}
	return nil
}

// pendingVersion is the version the next release publishes: the newest one
// that every program has an entry for and that no release has published. The
// version is fixed at promotion, so the release cannot pick a version the
// landed notes do not name.
func pendingVersion(published []releaseVersion) (releaseVersion, error) {
	changelogs := make([]releasenotes.Entries, 0, len(products))
	for _, p := range products {
		entries, err := p.embedded()
		if err != nil {
			return releaseVersion{}, err
		}
		changelogs = append(changelogs, entries)
	}
	return selectPendingVersion(changelogs, published)
}

func selectPendingVersion(changelogs []releasenotes.Entries, published []releaseVersion) (releaseVersion, error) {
	counts := make(map[string]int)
	for _, entries := range changelogs {
		for _, entry := range entries {
			if !entry.Draft {
				counts[entry.Version]++
			}
		}
	}

	newestPublished, hasPublished := newestVersion(published)
	var pending []releaseVersion
	for raw, count := range counts {
		version, err := parsePublishVersion(raw)
		if err != nil || count != len(changelogs) {
			continue
		}
		if hasPublished && compareVersions(version, newestPublished) <= 0 {
			continue
		}
		pending = append(pending, version)
	}
	version, ok := newestVersion(pending)
	if !ok {
		return releaseVersion{}, errors.New(
			"no promoted release notes are newer than the last release: run `/release-prep`, or `mise run changelog:promote` and `mise run changelog:pr`, and land the pull request")
	}
	return version, nil
}

// promoteDrafts collapses every product's accumulated fragments into its
// entry for version and deletes them, leaving each unreleased directory empty
// for the next cycle. It returns the entries it wrote.
func promoteDrafts(ctx context.Context, version releaseVersion) ([]string, error) {
	if err := validatePromoteSource(ctx); err != nil {
		return nil, err
	}
	return writePromotions(products, version, time.Now())
}

// writePromotions writes each product's entry for version from its draft.
//
// Every product gets an entry, including one with no fragments: the release
// ships every product under this version, and every product's changelog has
// to name every version it shipped under. That entry starts with an empty
// body, and its summary says the release changes nothing in that product.
//
// Each entry is the draft that builds have been showing, rendered the same
// way. It is written with an empty summary and is meant to be edited before it
// is committed: the draft is the sum of every pull request since the last
// release, and a changelog reads as what the program now does. Consolidating
// near-duplicate bullets and writing the one-line summary are this step's job
// (ADR release-notes-accumulate-as-fragments).
func writePromotions(products []product, version releaseVersion, date time.Time) ([]string, error) {
	type promotion struct {
		product   product
		body      string
		fragments []releasenotes.Fragment
	}
	promotions := make([]promotion, 0, len(products))
	total := 0
	for _, p := range products {
		entries, err := p.load(p.onDisk())
		if err != nil {
			return nil, err
		}
		fragments, err := releasenotes.Fragments(p.onDisk())
		if err != nil {
			return nil, fmt.Errorf("read %s changelog: %w", p.name, err)
		}
		if _, err := os.Stat(p.entryPath(version.String())); err == nil {
			return nil, fmt.Errorf("%s already exists", p.entryPath(version.String()))
		}
		draft, _ := entries.Draft()
		promotions = append(promotions, promotion{product: p, body: draft.Body, fragments: fragments})
		total += len(fragments)
	}
	if total == 0 {
		return nil, fmt.Errorf("no product has unreleased fragments: there is nothing to release")
	}

	header := fmt.Sprintf("---\nversion: %s\ndate: %s\nsummary: \"\"\n---\n\n",
		version, date.Format(time.DateOnly))
	paths := make([]string, 0, len(promotions))
	for _, promotion := range promotions {
		path := promotion.product.entryPath(version.String())
		contents := header
		if promotion.body != "" {
			contents += promotion.body + "\n"
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			return nil, fmt.Errorf("write changelog entry: %w", err)
		}
		for _, fragment := range promotion.fragments {
			if err := os.Remove(filepath.Join(promotion.product.unreleasedDir(), fragment.Name)); err != nil {
				return nil, fmt.Errorf("remove promoted fragment: %w", err)
			}
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// newFragment writes one unreleased change. The name is built here, never
// typed: a UTC timestamp and a slug of the note make it unique without
// allocating anything, so two branches writing release notes at the same time
// produce two files rather than one conflict.
func newFragment(p product, kind releasenotes.Kind, body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("a note is required")
	}
	slug := fragmentSlug(body)
	if slug == "" {
		return "", fmt.Errorf("note %q has no words to name the file after", body)
	}

	name := releasenotes.FragmentName(time.Now().UTC().Format(releasenotes.FragmentStampFormat), slug)
	dir := p.unreleasedDir()
	path := filepath.Join(dir, name)
	// git does not track an empty directory, so a checkout whose .gitkeep is
	// gone has no unreleased/ for the write to land in.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	contents := fmt.Sprintf("---\nkind: %s\n---\n\n%s\n", kind, body)
	if err := writeNewFile(path, []byte(contents)); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("%s already exists: a note that opens the same way was written this second; edit that one, or write this one again", path)
		}
		return "", fmt.Errorf("write changelog fragment: %w", err)
	}
	return path, nil
}

func writeNewFile(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// fragmentSlugWords bounds the slug: enough of the note to recognise it in a
// file listing, short enough that the name stays readable.
const fragmentSlugWords = 7

// fragmentSlug names a fragment after the opening of its note, with the
// markdown a note starts with — the bold lead-in, inline code — stripped.
func fragmentSlug(body string) string {
	var b strings.Builder
	prevDash := true
	for _, r := range strings.ToLower(body) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}

	words := strings.Split(strings.Trim(b.String(), "-"), "-")
	if len(words) > fragmentSlugWords {
		words = words[:fragmentSlugWords]
	}
	return strings.Join(words, "-")
}
