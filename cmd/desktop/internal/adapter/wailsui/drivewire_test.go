package wailsui

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app"
	"github.com/colonyops/hive/internal/domain/drive"
	drivesvc "github.com/colonyops/hive/internal/hive/drive"
)

func TestDriveWireTypesKeepTheirFieldNames(t *testing.T) {
	for _, tt := range []struct {
		name string
		v    any
		want []string
	}{
		{"SpaceWire", SpaceWire{}, sorted("kind", "key", "name")},
		{"EntryWire", EntryWire{}, sorted("path", "name", "kind", "size", "modified", "created", "title", "front", "children")},
		{"HitWire", HitWire{}, sorted("space", "entry")},
		{"DocumentWire", DocumentWire{}, sorted("space", "entry", "markdown", "html", "image")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, jsonKeys(t, tt.v))
		})
	}
}

func TestEntryWireOfConvertsTheWholeTree(t *testing.T) {
	modified := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	root := drive.Entry{
		Kind: drive.KindDir,
		Children: []drive.Entry{
			{Path: "plans", Name: "plans", Kind: drive.KindDir, Children: []drive.Entry{
				{Path: "plans/a.md", Name: "a.md", Kind: drive.KindMarkdown, Size: 12, Modified: modified, Title: "A", Front: drive.Frontmatter{"type": "plan"}},
			}},
		},
	}

	got := entryWireOf(root)
	require.Len(t, got.Children, 1)
	require.Len(t, got.Children[0].Children, 1)
	leaf := got.Children[0].Children[0]
	assert.Equal(t, EntryWire{
		Path: "plans/a.md", Name: "a.md", Kind: "markdown", Size: 12, Modified: modified, Title: "A",
		Front: map[string]any{"type": "plan"}, Children: []EntryWire{},
	}, leaf)

	raw, err := json.Marshal(leaf)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"children":[]`, "a leaf's children are an empty list, never null")
	assert.Contains(t, string(raw), `"modified":"2026-10-11T12:00:00Z"`)
	raw, err = json.Marshal(got)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"front":{}`, "absent front matter is an empty object, never null")
}

func TestDocumentWireOfCarriesOneBodyPerKind(t *testing.T) {
	space := drive.Shared
	md := documentWireOf(app.DocumentView{Space: space, Entry: drive.Entry{Path: "a.md", Kind: drive.KindMarkdown}, Markdown: "# A"})
	assert.Equal(t, SpaceWire{Kind: "shared", Key: "shared", Name: "Shared"}, md.Space)
	assert.Equal(t, "# A", md.Markdown)
	assert.Empty(t, md.HTML)
}

func TestHitWiresOfIsNeverNull(t *testing.T) {
	assert.Equal(t, []HitWire{}, hitWiresOf(nil))
	assert.Equal(t, []SpaceWire{}, spaceWiresOf(nil))

	hits := hitWiresOf([]drivesvc.Hit{{Space: drive.Shared, Entry: drive.Entry{Path: "x.md", Name: "x.md", Kind: drive.KindMarkdown}}})
	require.Len(t, hits, 1)
	assert.Equal(t, "shared", hits[0].Space.Key)
	assert.Equal(t, "x.md", hits[0].Entry.Path)
}
