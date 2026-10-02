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

// notesFor returns the release notes to publish with version.
//
// A stable release has an entry of its own, promoted from the draft before the
// release commit. A prerelease has none and carries the draft instead, which
// is exactly what its binary embeds — so the GitHub release body and the
// channel manifest say what the app itself will say.
func notesFor(version releaseVersion) (releasenotes.Entry, error) {
	entries, err := desktopProduct.embedded()
	if err != nil {
		return releasenotes.Entry{}, err
	}
	if entry, ok := entries.Find(version.String()); ok {
		if version.channel() == "stable" && entry.Summary == "" {
			return releasenotes.Entry{}, fmt.Errorf(
				"changelog entry for %s has no summary: it is what the What's New toast and the channel manifest show", version)
		}
		return entry, nil
	}
	if version.channel() == "stable" {
		return releasenotes.Entry{}, fmt.Errorf(
			"no changelog entry for %s: promote the drafts with `mise run changelog:promote -- %s` and commit it before releasing",
			version, version)
	}
	entry, _ := entries.Draft()
	return entry, nil
}

// validateChangelogEntry is the release gate. It runs with the other fail-fast
// checks, before anything is built: notes are embedded in the binary, so an
// entry written after the build would describe a release that cannot show it.
//
// Only a stable release is gated. A prerelease publishes whatever the draft
// says, including nothing — which is the point, since cutting one is meant to
// cost no changelog work at all.
func validateChangelogEntry(version releaseVersion) error {
	_, err := notesFor(version)
	return err
}

// releaseNotesBody assembles the GitHub release body: the R2 download header
// followed by the release notes. downloadBase is a parameter rather than a
// call to downloadBaseURL so the assembled body can be asserted against a
// literal, the same seam releaseNotesHeader has.
func releaseNotesBody(version releaseVersion, entry releasenotes.Entry, downloadBase string) string {
	body := releaseNotesHeader(version, downloadBase)
	if entry.Summary != "" {
		body += "\n" + entry.Summary + "\n"
	}
	return body + "\n" + entry.Body + "\n"
}

// promoteTargetVersion resolves the promote command's argument. "stable" picks
// the next stable version from the same sources planRelease uses — tags *and*
// the live manifests — because the R2 history predates this repository, and a
// promotion named off tags alone would write an entry the release then refuses
// to find.
func promoteTargetVersion(ctx context.Context, arg string) (releaseVersion, error) {
	if arg != "stable" {
		version, err := parsePublishVersion(arg)
		if err != nil {
			return releaseVersion{}, fmt.Errorf("invalid version %q: %w", arg, err)
		}
		if version.channel() != "stable" {
			return releaseVersion{}, fmt.Errorf(
				"%s is a prerelease: only a stable release gets an entry of its own, and a prerelease publishes the draft as it stands", version)
		}
		return version, nil
	}
	versions, _, err := releaseVersions(ctx)
	if err != nil {
		return releaseVersion{}, err
	}
	return parsePublishVersion(nextVersion("stable", versions))
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
