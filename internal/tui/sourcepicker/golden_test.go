package sourcepicker

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/colonyops/hive/internal/core/terminal"
	"github.com/colonyops/hive/internal/sources"
)

// goldenPicker builds a Picker in a deterministic state for golden
// snapshot tests: construct, run Init(), and drain its result synchronously.
func goldenPicker(t *testing.T, manifest sources.Manifest, items []sources.Item) Picker {
	t.Helper()
	fake := newFakeTUISource(manifest, items)
	p := newTestPicker(fake, manifest, "test-repo", 90, 24)
	return drainPicker(t, p, p.Init())
}

func TestPickerGolden_ListLayout(t *testing.T) {
	items := []sources.Item{
		{ID: "1", Title: "First reference item", Subtitle: "status: open", Fields: map[string]any{"number": 1278, "author": "alice", "labels": []string{"api", "public"}}},
		{ID: "2", Title: "Second reference item", Subtitle: "status: closed", Fields: map[string]any{"number": 1277, "author": "bob", "labels": []string{"backend"}}},
	}
	p := goldenPicker(t, listManifest(), items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_TableLayout(t *testing.T) {
	manifest := sources.Manifest{
		ID:          "fake",
		DisplayName: "Fake Table Source",
		Capabilities: sources.Capabilities{
			FetchDetail: false,
		},
		Picker: sources.PickerManifest{
			Layout: sources.LayoutModeTable,
			Columns: []sources.Column{
				{Key: "number", Label: "#", Width: 6},
				{Key: "title", Label: "Title", Width: 30},
				{Key: "state", Label: "State", Width: 10},
			},
			Search: sources.SearchManifest{Mode: sources.SearchModeLocal},
		},
	}
	items := []sources.Item{
		{ID: "1", Title: "Fix bug", Fields: map[string]any{"number": 1, "title": "Fix bug", "state": "open"}},
		{ID: "2", Title: "Add feature", Fields: map[string]any{"number": 2, "title": "Add feature", "state": "closed"}},
	}
	p := goldenPicker(t, manifest, items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_EmptyResults(t *testing.T) {
	p := goldenPicker(t, listManifest(), nil)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_ErrorState(t *testing.T) {
	fake := newFakeTUISource(listManifest(), nil)
	fake.searchErr = errGoldenDetail
	p := newTestPicker(fake, listManifest(), "test-repo", 90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

var errGoldenDetail = goldenDetailError("network unreachable")

type goldenDetailError string

func (e goldenDetailError) Error() string { return string(e) }

func TestPickerGolden_HidePreviewTable(t *testing.T) {
	manifest := sources.Manifest{
		ID:          "fake-prs",
		DisplayName: "Fake Pull Requests",
		Picker: sources.PickerManifest{
			Layout:      sources.LayoutModeTable,
			HidePreview: true,
			Columns: []sources.Column{
				{Key: "number", Label: "#", Width: 6},
				{Key: "title", Label: "Title", Flex: 1},
				{Key: "author", Label: "Author", Width: 14},
				{Key: "review", Label: "Review", Width: 18},
				{Key: "ci", Label: "CI", Width: 10},
			},
			Search: sources.SearchManifest{Mode: sources.SearchModeLocal},
		},
	}
	items := []sources.Item{
		{ID: "1315", Title: "feat(adaptivelogsapi): add pattern-match endpoint with a very long descriptive title that must not wrap", Fields: map[string]any{
			"number": 1315, "title": "feat(adaptivelogsapi): add pattern-match endpoint with a very long descriptive title that must not wrap", "author": "Johngeorgesample", "review": "approved", "ci": "passing",
		}},
		{ID: "1313", Title: "[DRAFT] OTel fleet pipeline indexed-label conversion parity with the gateway", Fields: map[string]any{
			"number": 1313, "title": "[DRAFT] OTel fleet pipeline indexed-label conversion parity with the gateway", "author": "MasslessParticle", "review": "draft", "ci": "pending",
		}},
		{ID: "1312", Title: "feat(fleet-otel): warn and emit a metric for unsupported regex index_label config", Fields: map[string]any{
			"number": 1312, "title": "feat(fleet-otel): warn and emit a metric for unsupported regex index_label config", "author": "MasslessParticle", "review": "open", "ci": "failing",
		}},
	}
	p := goldenPicker(t, manifest, items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}
