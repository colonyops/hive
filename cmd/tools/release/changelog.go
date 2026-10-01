package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/cmd/desktop/releasenotes"
)

// changelogDir is where an entry has to live to be embedded. The changelog
// sits inside the package that serves it because go:embed cannot reach above
// its own directory, the same arrangement flow/docs and actions/docs use.
const changelogDir = "cmd/desktop/releasenotes/changelog"

func unreleasedDir() string { return filepath.Join(changelogDir, releasenotes.UnreleasedDir) }

func entryPath(version releaseVersion) string {
	return filepath.Join(changelogDir, version.String()+".md")
}

// notesFor returns the release notes to publish with version.
//
// A stable release has an entry of its own, promoted from the draft before the
// release commit. A prerelease has none and carries the draft instead, which
// is exactly what its binary embeds — so the GitHub release body and the
// channel manifest say what the app itself will say.
//
// It reads through cmd/desktop/releasenotes — the same embedded changelog the
// app serves — rather than off disk, so a published version and the notes
// describing it cannot drift apart.
func notesFor(version releaseVersion) (releasenotes.Entry, error) {
	entries, err := releasenotes.Load()
	if err != nil {
		return releasenotes.Entry{}, fmt.Errorf("read changelog: %w", err)
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
			"no changelog entry for %s: promote the draft with `mise run desktop:changelog:promote -- %s` and commit it before releasing",
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

// promoteDraft collapses the accumulated fragments into version's entry and
// deletes them, leaving an empty unreleased directory for the next cycle.
//
// The entry it writes is the draft prereleases have been showing, rendered the
// same way. It is written with an empty summary and is meant to be edited
// before it is committed: the draft is the sum of every pull request since the
// last release, and a changelog reads as what the app now does. Consolidating
// near-duplicate bullets and writing the release's one-line summary are this
// step's job (ADR release-notes-accumulate-as-fragments).
func promoteDraft(ctx context.Context, version releaseVersion) (string, error) {
	if err := validatePromoteSource(ctx); err != nil {
		return "", err
	}
	entries, err := releasenotes.Load()
	if err != nil {
		return "", fmt.Errorf("read changelog: %w", err)
	}
	draft, ok := entries.Draft()
	if !ok {
		return "", fmt.Errorf("%s/ is empty: there is nothing to release", unreleasedDir())
	}
	fragments, err := releasenotes.Fragments()
	if err != nil {
		return "", fmt.Errorf("read changelog: %w", err)
	}

	path := entryPath(version)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}

	header := fmt.Sprintf("---\nversion: %s\ndate: %s\nsummary: \"\"\n---\n\n",
		version, time.Now().Format(time.DateOnly))
	if err := os.WriteFile(path, []byte(header+draft.Body+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write changelog entry: %w", err)
	}
	for _, fragment := range fragments {
		if err := os.Remove(filepath.Join(unreleasedDir(), fragment.Name)); err != nil {
			return "", fmt.Errorf("remove promoted fragment: %w", err)
		}
	}
	return path, nil
}

// newFragment writes one unreleased change. The name is built here, never
// typed: a UTC timestamp and a slug of the note make it unique without
// allocating anything, so two branches writing release notes at the same time
// produce two files rather than one conflict.
func newFragment(kind releasenotes.Kind, body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("a note is required")
	}
	slug := fragmentSlug(body)
	if slug == "" {
		return "", fmt.Errorf("note %q has no words to name the file after", body)
	}

	name := releasenotes.FragmentName(time.Now().UTC().Format(releasenotes.FragmentStampFormat), slug)
	path := filepath.Join(unreleasedDir(), name)
	// go:embed drops a directory holding only .gitkeep, so a checkout that
	// lost that file has no unreleased/ for the write to land in.
	if err := os.MkdirAll(unreleasedDir(), 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", unreleasedDir(), err)
	}
	contents := fmt.Sprintf("---\nkind: %s\n---\n\n%s\n", kind, body)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		return "", fmt.Errorf("write changelog fragment: %w", err)
	}
	return path, nil
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
