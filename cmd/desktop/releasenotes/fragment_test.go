package releasenotes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFragment(t *testing.T) {
	fragment, err := parseFragment("20260912T135003-a-thing.md",
		[]byte("---\nkind: added\n---\n\n**A thing.** It does something.\n"))
	require.NoError(t, err)

	assert.Equal(t, KindAdded, fragment.Kind)
	assert.Equal(t, "**A thing.** It does something.", fragment.Body)
}

func TestParseFragmentRejectsAMalformedName(t *testing.T) {
	for _, name := range []string{
		"a-thing.md",
		"20260912-a-thing.md",
		"20260912T135003.md",
		"20260912T135003-A-Thing.md",
		"20260912T1350-a-thing.md",
	} {
		_, err := parseFragment(name, []byte("---\nkind: added\n---\n\nbody\n"))
		assert.Error(t, err, "%q should be rejected", name)
	}
}

func TestParseFragmentRejectsAnUnknownKind(t *testing.T) {
	_, err := parseFragment("20260912T135003-a-thing.md",
		[]byte("---\nkind: removed\n---\n\nbody\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not one of")
}

func TestParseFragmentRequiresAHeaderAndABody(t *testing.T) {
	for name, raw := range map[string]string{
		"no frontmatter": "**A thing.** It does something.\n",
		"unterminated":   "---\nkind: added\n",
		"no kind":        "---\n\n---\n\nbody\n",
		"empty body":     "---\nkind: added\n---\n\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseFragment("20260912T135003-a-thing.md", []byte(raw))
			assert.Error(t, err)
		})
	}
}

func fragment(name string, kind Kind, body string) Fragment {
	return Fragment{Name: name, Kind: kind, Body: body}
}

// Sections come out in Kinds order however the fragments were read, and the
// filename timestamp orders the bullets inside one.
func TestRenderDraftGroupsByKindAndOrdersByName(t *testing.T) {
	body := renderDraft([]Fragment{
		fragment("20260912T090000-b.md", KindFixed, "**B.** fixed second."),
		fragment("20260912T080000-z.md", KindAdded, "**Z.** added second."),
		fragment("20260911T080000-a.md", KindFixed, "**A.** fixed first."),
		fragment("20260911T070000-y.md", KindAdded, "**Y.** added first."),
	})

	assert.Equal(t, `## Added

- **Y.** added first.
- **Z.** added second.

## Fixed

- **A.** fixed first.
- **B.** fixed second.`, body)
}

func TestRenderDraftOmitsAKindWithNoFragments(t *testing.T) {
	body := renderDraft([]Fragment{fragment("20260912T080000-a.md", KindChanged, "**A.** changed.")})

	assert.Equal(t, "## Changed\n\n- **A.** changed.", body)
}

func TestRenderDraftOfNothingIsEmpty(t *testing.T) {
	assert.Empty(t, renderDraft(nil))
}

// A note long enough to wrap has to stay inside its own bullet, or the
// continuation reads as a new paragraph after the list.
func TestRenderDraftIndentsContinuationLines(t *testing.T) {
	body := renderDraft([]Fragment{
		fragment("20260912T080000-a.md", KindAdded, "**A.** first line\nsecond line\n\nsecond paragraph"),
	})

	assert.Equal(t, "## Added\n\n- **A.** first line\n  second line\n\n  second paragraph", body)
}

// The committed fragments are the draft every prerelease build ships, so they
// are held to the same bar as a release entry.
func TestCommittedFragmentsParse(t *testing.T) {
	fragments, err := Fragments()
	require.NoError(t, err)

	for _, fragment := range fragments {
		assert.NotEmpty(t, fragment.Body, "%s has an empty body", fragment.Name)
		assert.Contains(t, Kinds, fragment.Kind, "%s has an unknown kind", fragment.Name)
	}
}

// Every branch open when the draft became a directory still edits next.md, and
// git merges that cleanly. The parse is the only thing that can say so.
func TestParseEntryExplainsAResurrectedNextMd(t *testing.T) {
	_, err := parseEntry("next.md", []byte("---\nsummary: \"\"\n---\n\n## Added\n\n- a thing\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), UnreleasedDir)
	assert.Contains(t, err.Error(), "changelog:new")
}
