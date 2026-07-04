package sourcepicker

import (
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/sources"
)

func TestRowWidthsMatchSelectedVsUnselected(t *testing.T) {
	p := newTestPicker(newFakeTUISource(listManifest(), nil), listManifest(), "test-repo", 90, 24)
	item := sources.Item{ID: "1", Title: "First reference item", Fields: map[string]any{"number": 1278, "author": "alice"}}
	tab := &tabState{tab: TabSource{ID: "fake", Manifest: listManifest()}}
	sel := p.renderRow(item, true, tab, 5)
	unsel := p.renderRow(item, false, tab, 5)
	assert.Equal(t, lipgloss.Width(unsel), lipgloss.Width(sel), "selected and unselected rows must have identical width")
	assert.Equal(t, p.innerWidth, lipgloss.Width(sel))
}
