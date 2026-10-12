package drive

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePath(t *testing.T) {
	for _, p := range []string{"plan.md", "plans/2026-10-11-plan.md", "a/b/c", "Reports/Q4.html", "with space/x.md"} {
		require.NoError(t, ValidatePath(p), p)
	}
	for _, p := range []string{
		"", "/", "/abs.md", "a/", "a//b", "..", "../x", "a/../b", ".", "./a", ".hive/x", "a/.git/b", "a/.x",
	} {
		require.ErrorIs(t, ValidatePath(p), ErrInvalidPath, p)
	}
}

func TestSortEntriesDirsFirstThenNatural(t *testing.T) {
	entries := []Entry{
		{Name: "zeta.md", Kind: KindMarkdown},
		{Name: "10-ten.md", Kind: KindMarkdown},
		{Name: "research", Kind: KindDir},
		{Name: "2-two.md", Kind: KindMarkdown},
		{Name: "Alpha.md", Kind: KindMarkdown},
		{Name: "beta.html", Kind: KindHtml},
		{Name: "canvases", Kind: KindDir},
		{Name: "file2.md", Kind: KindMarkdown},
		{Name: "file10.md", Kind: KindMarkdown},
		{Name: "file1.md", Kind: KindMarkdown},
		{Name: "Plans", Kind: KindDir},
	}
	SortEntries(entries)

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	assert.Equal(t, []string{
		"canvases", "Plans", "research",
		"2-two.md", "10-ten.md", "Alpha.md", "beta.html", "file1.md", "file2.md", "file10.md", "zeta.md",
	}, names)
}

func TestSortEntriesIsDeterministicAcrossCase(t *testing.T) {
	entries := []Entry{{Name: "b.md"}, {Name: "B.md"}, {Name: "a.md"}}
	SortEntries(entries)
	assert.Equal(t, []Entry{{Name: "a.md"}, {Name: "B.md"}, {Name: "b.md"}}, entries)
}

func TestFind(t *testing.T) {
	root := Entry{Kind: KindDir, Children: []Entry{
		{Path: "plans", Name: "plans", Kind: KindDir, Children: []Entry{
			{Path: "plans/x.md", Name: "x.md", Kind: KindMarkdown},
		}},
		{Path: "readme.md", Name: "readme.md", Kind: KindMarkdown},
	}}

	got, ok := Find(root, "")
	require.True(t, ok)
	assert.Equal(t, root, got)

	got, ok = Find(root, "plans/x.md")
	require.True(t, ok)
	assert.Equal(t, "plans/x.md", got.Path)

	got, ok = Find(root, "plans/")
	require.True(t, ok)
	assert.Equal(t, "plans", got.Path)

	for _, p := range []string{"plans/y.md", "readme.md/x", "Plans/x.md", "nope"} {
		_, ok := Find(root, p)
		assert.False(t, ok, p)
	}
}
