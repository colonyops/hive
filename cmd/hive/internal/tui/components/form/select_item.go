package form

// selectItem is the list item used by select and multi-select fields.
type selectItem struct {
	label string
	index int
}

func (i selectItem) FilterValue() string { return i.label }
