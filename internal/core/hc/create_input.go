package hc

// CreateInput is declared in create_input.gen.go, generated from
// internal/schema/hc/tree.cue.

// Count returns the number of nodes in the tree, the root included.
func (c CreateInput) Count() int {
	n := 1
	for _, child := range c.Children {
		n += child.Count()
	}
	return n
}

// CreateItemInput describes a single-item create request.
type CreateItemInput struct {
	Title    string
	Desc     string
	Type     ItemType
	ParentID string
}
