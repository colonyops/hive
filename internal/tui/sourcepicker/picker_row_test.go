package sourcepicker

import (
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"github.com/colonyops/hive/internal/sources"
)

func TestRowWidthsMatchSelectedVsUnselected(t *testing.T) {
	p := newTestPicker(newFakeTUISource(fakeManifest(), nil), fakeManifest(), "test-repo", 90, 24)
	item := sources.Item{ID: "1", Title: "First reference item", Fields: map[string]any{"number": 1278, "author": "alice"}}
	sel := p.renderRow(item, true, 5)
	unsel := p.renderRow(item, false, 5)
	assert.Equal(t, lipgloss.Width(unsel), lipgloss.Width(sel), "selected and unselected rows must have identical width")
	assert.Equal(t, p.innerWidth, lipgloss.Width(sel))
}
