package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/internal/releasenotes"
)

// releaseEntry returns p's entry for version, as p's binary embeds it.
//
// Every release has an entry per program, promoted from the drafts before the
// release commit. Reading through the embed means the GitHub release body and
// the update manifest say what the binary itself will say.
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
// checks, before anything is built: notes are embedded in the binaries, so an
// entry written after the build would describe a release that cannot show it.
func validateChangelogEntry(version releaseVersion) error {
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
// program's heading.
func demoteHeadings(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "#") {
			lines[i] = "#" + line
		}
	}
	return strings.Join(lines, "\n")
}

// promoteTargetVersion resolves the promote command's optional argument. With
// none it picks the next date version from the same sources planRelease uses,
// tags and the live manifests, because the R2 history predates this
// repository.
func promoteTargetVersion(ctx context.Context, arg string) (releaseVersion, error) {
	published, _, err := releaseVersions(ctx)
	if err != nil {
		return releaseVersion{}, err
	}
	if arg == "" {
		return nextVersion(time.Now(), published)
	}
	version, err := parsePublishVersion(arg)
	if err != nil {
		return releaseVersion{}, fmt.Errorf("invalid version %q: %w", arg, err)
	}
	if newest, ok := newestVersion(published); ok && compareVersions(version, newest) <= 0 {
		return releaseVersion{}, fmt.Errorf("%s does not advance the newest published version %s", version, newest)
	}
	return version, nil
}

// pendingVersion is the version the next release publishes: the newest one
// that every program has an entry for and that no release has published. The
// version is fixed at promotion rather than on the release day, so release
// notes that land after midnight still release under their own version.
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
