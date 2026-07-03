package connectorpicker

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/terminal"
)

// goldenPicker builds a Picker in a deterministic state for golden
// snapshot tests: construct, run Init(), and drain its result synchronously.
func goldenPicker(t *testing.T, manifest connectors.Manifest, items []connectors.Item) Picker {
	t.Helper()
	fake := newFakeTUIConnector(manifest, items)
	p := New(fake, manifest, "", 80, 24)
	p.SetSize(90, 24)
	return drainPicker(t, p, p.Init())
}

func TestPickerGolden_ListLayout(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "First reference item", Subtitle: "status: open", Fields: map[string]any{"number": 1278, "author": "alice", "labels": []string{"api", "public"}}},
		{ID: "2", Title: "Second reference item", Subtitle: "status: closed", Fields: map[string]any{"number": 1277, "author": "bob", "labels": []string{"backend"}}},
	}
	p := goldenPicker(t, listManifest(), items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_TableLayout(t *testing.T) {
	manifest := connectors.Manifest{
		ID:          "fake",
		DisplayName: "Fake Table Connector",
		Capabilities: connectors.Capabilities{
			FetchDetail: false,
		},
		Picker: connectors.PickerManifest{
			Layout: connectors.LayoutModeTable,
			Columns: []connectors.Column{
				{Key: "number", Label: "#", Width: 6},
				{Key: "title", Label: "Title", Width: 30},
				{Key: "state", Label: "State", Width: 10},
			},
			Search: connectors.SearchManifest{Mode: connectors.SearchModeLocal},
		},
	}
	items := []connectors.Item{
		{ID: "1", Title: "Fix bug", Fields: map[string]any{"number": 1, "title": "Fix bug", "state": "open"}},
		{ID: "2", Title: "Add feature", Fields: map[string]any{"number": 2, "title": "Add feature", "state": "closed"}},
	}
	p := goldenPicker(t, manifest, items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_MarkdownDetail(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "Item with markdown detail", Fields: map[string]any{"number": 42, "author": "alice", "labels": []string{"docs", "preview"}}},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detail["1"] = connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "# Heading\n\nSome body text."}}
	p := New(fake, listManifest(), "", 80, 24)
	p.SetSize(90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_KVDetail(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "Item with kv detail"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detail["1"] = connectors.Detail{KV: &connectors.KVDetail{
		Sections: []connectors.KVSection{
			{
				Heading: "Metadata",
				Pairs: []connectors.KVPair{
					{Key: "status", Value: "open"},
					{Key: "owner", Value: "alice"},
				},
			},
		},
	}}
	p := New(fake, listManifest(), "", 80, 24)
	p.SetSize(90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_EmptyResults(t *testing.T) {
	p := goldenPicker(t, listManifest(), nil)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_DetailError(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "Item whose detail fails to load"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detailErr["1"] = errGoldenDetail
	p := New(fake, listManifest(), "", 80, 24)
	p.SetSize(90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

var errGoldenDetail = goldenDetailError("network unreachable")

type goldenDetailError string

func (e goldenDetailError) Error() string { return string(e) }

// TestPickerGolden_HidePreviewTable snapshots the single-pane
// (HidePreview) table layout used by the built-in PR connector: the table
// gets the full modal width, flex columns absorb the remaining space, and
// long titles truncate on one line instead of wrapping.
func TestPickerGolden_HidePreviewTable(t *testing.T) {
	manifest := connectors.Manifest{
		ID:          "fake-prs",
		DisplayName: "Fake Pull Requests",
		Picker: connectors.PickerManifest{
			Layout:      connectors.LayoutModeTable,
			HidePreview: true,
			Columns: []connectors.Column{
				{Key: "number", Label: "#", Width: 6},
				{Key: "title", Label: "Title", Flex: 1},
				{Key: "author", Label: "Author", Width: 14},
				{Key: "review", Label: "Review", Width: 18},
			},
			Search: connectors.SearchManifest{Mode: connectors.SearchModeLocal},
		},
	}
	items := []connectors.Item{
		{ID: "1315", Title: "feat(adaptivelogsapi): add pattern-match endpoint with a very long descriptive title that must not wrap", Fields: map[string]any{
			"number": 1315, "title": "feat(adaptivelogsapi): add pattern-match endpoint with a very long descriptive title that must not wrap", "author": "Johngeorgesample", "review": "approved",
		}},
		{ID: "1313", Title: "[DRAFT] OTel fleet pipeline indexed-label conversion parity with the gateway", Fields: map[string]any{
			"number": 1313, "title": "[DRAFT] OTel fleet pipeline indexed-label conversion parity with the gateway", "author": "MasslessParticle", "review": "draft",
		}},
		{ID: "1312", Title: "feat(fleet-otel): warn and emit a metric for unsupported regex index_label config", Fields: map[string]any{
			"number": 1312, "title": "feat(fleet-otel): warn and emit a metric for unsupported regex index_label config", "author": "MasslessParticle", "review": "open",
		}},
	}
	p := goldenPicker(t, manifest, items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}
