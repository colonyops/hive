package hc

// CreateInput represents a node in a hierarchical task tree for bulk creation.
// The tree is walked BFS and IDs are generated in the service layer.
type CreateInput struct {
	Title    string        `json:"title"`
	Desc     string        `json:"desc,omitempty"`
	Type     ItemType      `json:"type"`
	Children []CreateInput `json:"children,omitempty"`
}
