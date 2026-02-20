package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoDialog_RendersSectionsItemsFooter(t *testing.T) {
	d := NewInfoDialog(
		"Hive Info",
		[]InfoSection{
			{
				Title: "Build",
				Items: []InfoItem{
					{Label: "Version", Value: "dev"},
					{Label: "Commit", Value: "abc1234"},
				},
			},
			{
				Title: "Checks",
				Items: []InfoItem{
					{Label: "git", Value: "ok", Status: InfoStatusPass},
					{Label: "tmux", Value: "warn", Status: InfoStatusWarn},
					{Label: "cfg", Value: "bad", Status: InfoStatusFail},
				},
			},
		},
		"footer summary",
		"[j/k] scroll  [esc] close",
		120,
		40,
	)

	out := d.Overlay("bg", 120, 40)
	assert.Contains(t, out, "Hive Info")
	assert.Contains(t, out, "Build")
	assert.Contains(t, out, "Version")
	assert.Contains(t, out, "abc1234")
	assert.Contains(t, out, "footer summary")
	assert.Contains(t, out, "✔")
	assert.Contains(t, out, "●")
	assert.Contains(t, out, "✘")
}

func TestInfoDialog_ScrollAndEmptySections(t *testing.T) {
	items := make([]InfoItem, 0, 50)
	for i := 0; i < 50; i++ {
		items = append(items, InfoItem{Label: "item", Value: "value"})
	}

	d := NewInfoDialog(
		"Hive Doctor",
		[]InfoSection{
			{Title: "Many", Items: items},
			{Title: "Empty", Items: nil},
		},
		"",
		"help",
		70,
		18,
	)

	before := d.Overlay("bg", 70, 18)
	d.ScrollDown()
	after := d.Overlay("bg", 70, 18)

	assert.Contains(t, before, "Hive Doctor")
	assert.Contains(t, after, "Hive Doctor")
	assert.NotEqual(t, before, after)
}
