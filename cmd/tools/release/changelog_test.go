package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/colonyops/hive/internal/releasenotes"
)

// The gate that stands between the drafts and a release: every program's
// entry has to be promoted, under the version's own name, before publishing.
func TestValidateChangelogEntryRequiresAPromotedEntry(t *testing.T) {
	err := validateChangelogEntry(mustVersion(t, "99.0.0"))
	if err == nil {
		t.Fatal("expected a release with no entry to be rejected")
	}
	if !strings.Contains(err.Error(), "changelog:promote") {
		t.Fatalf("error should name the promote command, got %q", err)
	}
}

func TestRenderReleaseNotesBody(t *testing.T) {
	version := mustVersion(t, "0.20261001.0")
	body := renderReleaseNotesBody(version, "https://dl.hivedesktop.com", []productNotes{
		{product: cliProduct, entry: releasenotes.Entry{Summary: "No changes to the hive CLI."}},
		{product: desktopProduct, entry: releasenotes.Entry{Summary: "A short line.", Body: "## Added\n\n- a thing"}},
	})

	want := releaseNotesHeader(version, "https://dl.hivedesktop.com") +
		"\n## hive CLI\n\nNo changes to the hive CLI.\n" +
		"\n## Hive Desktop\n\nA short line.\n\n### Added\n\n- a thing\n"
	if body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func changelog(versions ...string) releasenotes.Entries {
	entries := releasenotes.Entries{{Draft: true}}
	for _, version := range versions {
		entries = append(entries, releasenotes.Entry{Version: version})
	}
	return entries
}

func TestSelectPendingVersion(t *testing.T) {
	published := []releaseVersion{mustParseVersion(t, "0.59.0"), mustParseVersion(t, "0.20261001.0")}

	tests := []struct {
		name       string
		changelogs []releasenotes.Entries
		want       string
		wantErr    string
	}{
		{
			name:       "the newest entry every program has",
			changelogs: []releasenotes.Entries{changelog("0.20261001.0", "0.20261002.0"), changelog("0.9.0", "0.20261001.0", "0.20261002.0")},
			want:       "0.20261002.0",
		},
		{
			name:       "an entry one program lacks is not pending",
			changelogs: []releasenotes.Entries{changelog("0.20261002.0", "0.20261003.0"), changelog("0.20261002.0")},
			want:       "0.20261002.0",
		},
		{
			name:       "an entry that is already published is not pending",
			changelogs: []releasenotes.Entries{changelog("0.20261001.0"), changelog("0.20261001.0")},
			wantErr:    "no promoted release notes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectPendingVersion(test.changelogs, published)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("selectPendingVersion() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != test.want {
				t.Fatalf("selectPendingVersion() = %s, want %s", got, test.want)
			}
		})
	}
}

// The slug is what makes two fragments written in the same second distinct, and
// what makes a file listing readable, so it has to survive the markdown a note
// opens with.
func TestFragmentSlug(t *testing.T) {
	for note, want := range map[string]string{
		"**A notify terminal node**, so a feed can notify on new items.": "a-notify-terminal-node-so-a-feed",
		"**Refresh now fetches.**":                                       "refresh-now-fetches",
		"`profiles.order` is read":                                       "profiles-order-is-read",
		"Settings ▸ Terminal opens":                                      "settings-terminal-opens",
		"one":                                                            "one",
	} {
		if got := fragmentSlug(note); got != want {
			t.Errorf("fragmentSlug(%q) = %q, want %q", note, got, want)
		}
	}
}

// A name the tool builds has to be a name the parser accepts, or a fragment
// lands that no build can read.
func TestFragmentSlugProducesAParseableName(t *testing.T) {
	for _, note := range []string{
		"**A thing.** It does something.",
		"`code` and ▸ symbols -- and punctuation!",
		"123 numeric lead",
	} {
		name := releasenotes.FragmentName("20260912T135003", fragmentSlug(note))
		if err := releasenotes.CheckFragmentName(name); err != nil {
			t.Errorf("fragment name for %q: %v", note, err)
		}
	}
}

func TestNewFragmentRejectsANoteWithNoWords(t *testing.T) {
	if _, err := newFragment(cliProduct, releasenotes.KindAdded, "   "); err == nil {
		t.Fatal("expected an empty note to be rejected")
	}
	if _, err := newFragment(cliProduct, releasenotes.KindAdded, "***"); err == nil {
		t.Fatal("expected a note with no words to be rejected")
	}
}

func TestParseProduct(t *testing.T) {
	for _, p := range products {
		got, err := parseProduct(p.name)
		if err != nil || got.dir != p.dir {
			t.Errorf("parseProduct(%q) = %+v, %v", p.name, got, err)
		}
	}
	if _, err := parseProduct("relay"); err == nil {
		t.Fatal("expected an unknown product to be rejected")
	}
}

// The release tool writes into a product's directory and reads it back
// through the product's embed, so the two have to name the same tree.
func TestProductDirMatchesItsEmbed(t *testing.T) {
	for _, p := range products {
		onDisk, err := releasenotes.Load(os.DirFS(filepath.Join("..", "..", "..", p.dir)))
		if err != nil {
			t.Fatalf("%s: load from disk: %v", p.name, err)
		}
		embedded, err := p.embedded()
		if err != nil {
			t.Fatalf("%s: load embed: %v", p.name, err)
		}
		if !reflect.DeepEqual(onDisk, embedded) {
			t.Errorf("%s: %s and the embed hold different entries:\n%+v\n%+v", p.name, p.dir, onDisk, embedded)
		}
	}
}

func TestWriteNewFileRefusesToOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := writeNewFile(path, []byte("first")); err != nil {
		t.Fatal(err)
	}

	err := writeNewFile(path, []byte("second"))
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("second write: got %v, want fs.ErrExist", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "first" {
		t.Fatalf("first write was replaced: %q", raw)
	}
}

// Two notes that open the same way, written in the same second, build the same
// name. The name holds the current second, so the test occupies the name for
// this second and the next instead of racing the clock.
func TestNewFragmentNamesTheFileItRefusedToReplace(t *testing.T) {
	p := product{name: "test", dir: t.TempDir()}
	if err := os.MkdirAll(p.unreleasedDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	const note = "**The same note.** second"
	now := time.Now().UTC()
	for _, stamp := range []time.Time{now, now.Add(time.Second)} {
		name := releasenotes.FragmentName(stamp.Format(releasenotes.FragmentStampFormat), fragmentSlug(note))
		if err := os.WriteFile(filepath.Join(p.unreleasedDir(), name), []byte("first"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := newFragment(p, releasenotes.KindFixed, note)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected the second write to be refused by name, got %v", err)
	}
}

func testProduct(t *testing.T, name string, fragments map[string]string) product {
	t.Helper()
	p := product{name: name, title: name, dir: t.TempDir()}
	if err := os.MkdirAll(p.unreleasedDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for file, body := range fragments {
		contents := "---\nkind: added\n---\n\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(p.unreleasedDir(), file), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestWritePromotionsWritesOneEntryPerProduct(t *testing.T) {
	version := mustVersion(t, "0.5.0")
	changed := testProduct(t, "changed", map[string]string{"20260912T135002-a-thing.md": "**A thing.**"})
	untouched := testProduct(t, "untouched", nil)

	paths, err := writePromotions([]product{changed, untouched}, version, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{changed.entryPath("0.5.0"), untouched.entryPath("0.5.0")}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}

	for p, wantBody := range map[product]string{changed: "## Added\n\n- **A thing.**", untouched: ""} {
		entries, err := releasenotes.Load(p.onDisk())
		if err != nil {
			t.Fatalf("%s: %v", p.name, err)
		}
		entry, ok := entries.Find("0.5.0")
		if !ok {
			t.Fatalf("%s: no entry for 0.5.0 in %+v", p.name, entries)
		}
		if entry.Summary != "" || entry.Body != wantBody || entry.Date.Format(time.DateOnly) != "2026-10-01" {
			t.Fatalf("%s: entry = %+v", p.name, entry)
		}
		if _, ok := entries.Draft(); ok {
			t.Fatalf("%s: the fragments were not deleted", p.name)
		}
	}
}

func TestWritePromotionsRefusesAnExistingEntryBeforeWritingAnything(t *testing.T) {
	version := mustVersion(t, "0.5.0")
	first := testProduct(t, "first", map[string]string{"20260912T135002-a-thing.md": "**A thing.**"})
	second := testProduct(t, "second", map[string]string{"20260912T135003-another.md": "**Another.**"})
	if err := os.WriteFile(second.entryPath("0.5.0"), []byte("---\nversion: 0.5.0\ndate: 2026-01-01\nsummary: Old.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := writePromotions([]product{first, second}, version, time.Now())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected the existing entry to be refused, got %v", err)
	}
	if _, err := os.Stat(first.entryPath("0.5.0")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the first product's entry was written before the refusal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(first.unreleasedDir(), "20260912T135002-a-thing.md")); err != nil {
		t.Fatalf("the first product's fragment was deleted before the refusal: %v", err)
	}
}

func TestWritePromotionsRefusesWhenNoProductHasFragments(t *testing.T) {
	products := []product{testProduct(t, "a", nil), testProduct(t, "b", nil)}

	_, err := writePromotions(products, mustVersion(t, "0.5.0"), time.Now())
	if err == nil || !strings.Contains(err.Error(), "nothing to release") {
		t.Fatalf("expected an empty promotion to be refused, got %v", err)
	}
}
