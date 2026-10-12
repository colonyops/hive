package review

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/internal/domain/drive"
)

func md(path string, modified time.Time, front drive.Frontmatter) drive.Entry {
	return drive.Entry{Path: path, Name: path, Kind: drive.KindMarkdown, Modified: modified, Front: front}
}

func repoSpace(t *testing.T) drive.Space {
	t.Helper()
	space, err := drive.RepositorySpace("acme", "site")
	require.NoError(t, err)
	return space
}

func TestDocumentsFromTree_Typing(t *testing.T) {
	now := time.Now()
	root := drive.Entry{Kind: drive.KindDir, Children: []drive.Entry{
		{Path: "plans", Kind: drive.KindDir, Children: []drive.Entry{
			md("plans/a.md", now, nil),
			md("plans/design.md", now, drive.Frontmatter{"type": "design-doc"}),
			md("plans/untyped.md", now, drive.Frontmatter{"type": 3}),
		}},
		md("loose.md", now, nil),
		{Path: "notes.txt", Kind: drive.KindFile},
		{Path: "board.canvas.json", Kind: drive.KindCanvas},
	}}

	docs := DocumentsFromTree(repoSpace(t), "/ctx/acme/site", root, false)

	got := map[string]string{}
	for _, d := range docs {
		got[d.RelPath] = d.Type
	}
	assert.Equal(t, map[string]string{
		".hive/plans/a.md":       "plan",
		".hive/plans/design.md":  "design-doc",
		".hive/plans/untyped.md": "plan",
		".hive/loose.md":         "other",
	}, got, "front matter type wins, then the top folder, then other; only markdown is listed")
}

func TestDocumentsFromTree_Paths(t *testing.T) {
	root := drive.Entry{Kind: drive.KindDir, Children: []drive.Entry{md("plans/a.md", time.Now(), nil)}}

	repo := DocumentsFromTree(repoSpace(t), "/ctx/acme/site", root, false)
	require.Len(t, repo, 1)
	assert.Equal(t, "/ctx/acme/site/plans/a.md", repo[0].Path)
	assert.Equal(t, ".hive/plans/a.md", repo[0].RelPath)

	shared := DocumentsFromTree(drive.Shared, "/ctx/shared", root, false)
	assert.Equal(t, "plans/a.md", shared[0].RelPath)

	all := DocumentsFromTree(repoSpace(t), "/ctx/acme/site", root, true)
	assert.Equal(t, "acme/site/plans/a.md", all[0].RelPath)
	assert.Equal(t, repoSpace(t), all[0].Space)
}

func TestDocumentsFromTree_SortOrder(t *testing.T) {
	now := time.Now()
	older := now.Add(-time.Hour)
	newest := now.Add(time.Hour)
	typed := func(path, typ string, at time.Time) drive.Entry {
		return md(path, at, drive.Frontmatter{"type": typ})
	}
	root := drive.Entry{Kind: drive.KindDir, Children: []drive.Entry{
		md("loose.md", newest, nil),
		typed("a.md", "zebra", now),
		typed("b.md", "context", now),
		typed("c.md", "plan", older),
		typed("d.md", "apple", now),
		typed("e.md", "reference", now),
		typed("f.md", "plan", newest),
		typed("g.md", "review", now),
		typed("h.md", "research", now),
		typed("i.md", "design-doc", now),
	}}

	docs := DocumentsFromTree(drive.Shared, "/ctx/shared", root, false)

	var order []string
	for _, d := range docs {
		order = append(order, d.Path[len("/ctx/shared/"):])
	}
	assert.Equal(t, []string{
		"f.md", "c.md", // plan, newest first
		"i.md",     // design-doc
		"h.md",     // research
		"g.md",     // review
		"e.md",     // reference
		"b.md",     // context
		"d.md",     // apple
		"a.md",     // zebra
		"loose.md", // other
	}, order)
}

func TestTypeTitle(t *testing.T) {
	assert.Equal(t, "Plan", typeTitle("plan"))
	assert.Equal(t, "Design doc", typeTitle("design-doc"))
	assert.Equal(t, "Other", typeTitle(""))
}

func TestTypeFromPath(t *testing.T) {
	assert.Equal(t, "plan", typeFromPath("plans/x.md"))
	assert.Equal(t, "research", typeFromPath(".hive/research/x.md"))
	assert.Equal(t, "notes", typeFromPath("notes/x.md"), "any other folder is its own type")
	assert.Equal(t, "other", typeFromPath("x.md"))
}
