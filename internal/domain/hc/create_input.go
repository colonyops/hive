package hc

// CreateInput represents a node in a hierarchical task tree for bulk creation.
// The tree is walked BFS and IDs are generated in the service layer.
type CreateInput struct {
	Ref      string        `json:"ref,omitempty"` // optional local reference for blocker wiring
	Title    string        `json:"title"`
	Desc     string        `json:"desc,omitempty"`
	Type     ItemType      `json:"type"`
	Children []CreateInput `json:"children,omitempty"`
	Blockers []string      `json:"blockers,omitempty"` // refs of items that must complete before this one
}

// CreateItemInput describes a single-item create request.
type CreateItemInput struct {
	Title    string
	Desc     string
	Type     ItemType
	ParentID string
}
