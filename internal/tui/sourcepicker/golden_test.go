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

func TestPickerGolden_EmptyResults(t *testing.T) {
	p := goldenPicker(t, fakeManifest(), nil)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

func TestPickerGolden_ErrorState(t *testing.T) {
	fake := newFakeTUISource(fakeManifest(), nil)
	fake.searchErr = errGoldenDetail
	p := newTestPicker(fake, fakeManifest(), "test-repo", 90, 24)
	p = drainPicker(t, p, p.Init())
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}

var errGoldenDetail = goldenDetailError("network unreachable")

type goldenDetailError string

func (e goldenDetailError) Error() string { return string(e) }

func TestPickerGolden_CardLayout(t *testing.T) {
	manifest := sources.Manifest{
		ID:          "fake-prs-card",
		DisplayName: "Fake Pull Requests",
	}
	items := []sources.Item{
		{ID: "366", Title: "feat: connectors vertical slice with a title long enough to truncate on line one", Fields: map[string]any{
			"number": 366, "author": "hay-kot", "review": "draft", "ci": "pending", "labels": []string{"tui", "sources"},
			"age": "3w", "linked_issue": 342, "linked_issue_count": 1, "assignee": "hay-kot", "assignee_count": 1,
		}},
		{ID: "359", Title: "fix: stop KV filter from swallowing keys on other views", Fields: map[string]any{
			"number": 359, "author": "hay-kot", "review": "approved", "ci": "passing", "labels": []string{"bug"},
			"age": "5d",
		}},
		{ID: "351", Title: "refactor: source registry ordering", Fields: map[string]any{
			"number": 351, "author": "contributor", "review": "changes requested", "ci": "failing",
			"age": "2d", "assignee": "alice", "assignee_count": 3,
		}},
	}
	p := goldenPicker(t, manifest, items)
	golden.RequireEqual(t, []byte(terminal.StripANSI(p.View())))
}
