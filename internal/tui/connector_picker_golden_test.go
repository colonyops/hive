package tui

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/colonyops/hive/internal/connectors"
	"github.com/colonyops/hive/internal/core/terminal"
)

// goldenPicker builds a ConnectorPicker in a deterministic state for golden
// snapshot tests: construct, run Init(), and drain its result synchronously.
func goldenPicker(t *testing.T, manifest connectors.Manifest, items []connectors.Item) ConnectorPicker {
	t.Helper()
	fake := newFakeTUIConnector(manifest, items)
	p := NewConnectorPicker(fake, manifest, "")
	p.SetSize(90, 24)
	return drainPicker(t, p, p.Init())
}

func TestConnectorPickerGolden_ListLayout(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "First reference item", Subtitle: "status: open"},
		{ID: "2", Title: "Second reference item", Subtitle: "status: closed"},
	}
	p := goldenPicker(t, listManifest(), items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestConnectorPickerGolden_TableLayout(t *testing.T) {
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

func TestConnectorPickerGolden_MarkdownDetail(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "Item with markdown detail"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detail["1"] = connectors.Detail{Markdown: &connectors.MarkdownDetail{Content: "# Heading\n\nSome body text."}}
	p := NewConnectorPicker(fake, listManifest(), "")
	p.SetSize(90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestConnectorPickerGolden_KVDetail(t *testing.T) {
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
	p := NewConnectorPicker(fake, listManifest(), "")
	p.SetSize(90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestConnectorPickerGolden_EmptyResults(t *testing.T) {
	p := goldenPicker(t, listManifest(), nil)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestConnectorPickerGolden_DetailError(t *testing.T) {
	items := []connectors.Item{
		{ID: "1", Title: "Item whose detail fails to load"},
	}
	fake := newFakeTUIConnector(listManifest(), items)
	fake.detailErr["1"] = errGoldenDetail
	p := NewConnectorPicker(fake, listManifest(), "")
	p.SetSize(90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

var errGoldenDetail = goldenDetailError("network unreachable")

type goldenDetailError string

func (e goldenDetailError) Error() string { return string(e) }
