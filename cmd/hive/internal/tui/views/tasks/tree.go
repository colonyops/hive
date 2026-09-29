package tasks

import (
	"sort"

	"github.com/colonyops/hive/internal/core/hc"
)

// TreeNode represents an item in the tree hierarchy.
type TreeNode struct {
	Item     hc.Item
	Children []*TreeNode
	Expanded bool
}

// FlatNode is a flattened tree node for rendering.
type FlatNode struct {
	Node   *TreeNode
	Depth  int
	IsLast bool // last child of parent
}

// buildTree groups items by parent and builds a hierarchy.
// Epics/root items become roots, sorted by CreatedAt DESC.
// Children maintain input order. Epics start expanded.
func buildTree(items []hc.Item) []*TreeNode {
	if len(items) == 0 {
		return nil
	}

	byID := make(map[string]*TreeNode, len(items))
	for i := range items {
		byID[items[i].ID] = &TreeNode{
			Item:     items[i],
			Expanded: items[i].Type == hc.ItemTypeEpic,
		}
	}

	children := make(map[string][]*TreeNode)
	var roots []*TreeNode

	for _, item := range items {
		node := byID[item.ID]
		if _, hasParent := byID[item.ParentID]; !hasParent {
			roots = append(roots, node)
		} else {
			children[item.ParentID] = append(children[item.ParentID], node)
		}
	}

	// Attach children to parent nodes.
	for parentID, kids := range children {
		if parent, ok := byID[parentID]; ok {
			parent.Children = kids
		}
	}

	// Sort roots by CreatedAt DESC (newest first).
	sort.SliceStable(roots, func(i, j int) bool {
		return roots[i].Item.CreatedAt.After(roots[j].Item.CreatedAt)
	})

	return roots
}

// flattenTree flattens expanded nodes into a renderable list.
func flattenTree(roots []*TreeNode) []FlatNode {
	var result []FlatNode

	var walk func(nodes []*TreeNode, depth int)
	walk = func(nodes []*TreeNode, depth int) {
		for i, node := range nodes {
			isLast := i == len(nodes)-1
			result = append(result, FlatNode{
				Node:   node,
				Depth:  depth,
				IsLast: isLast,
			})
			if node.Expanded && len(node.Children) > 0 {
				walk(node.Children, depth+1)
			}
		}
	}

	walk(roots, 0)
	return result
}

// countByStatus counts done vs total leaf descendants recursively.
func countByStatus(children []*TreeNode) (done, total int) {
	for _, child := range children {
		if len(child.Children) > 0 {
			d, t := countByStatus(child.Children)
			done += d
			total += t
		} else {
			total++
			if child.Item.Status == hc.StatusDone {
				done++
			}
		}
	}
	return done, total
}
