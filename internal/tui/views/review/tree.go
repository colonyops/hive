package review

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DocTreeNode represents a node in the document folder tree.
// It can be either a directory node or a document leaf node.
type DocTreeNode struct {
	Name     string         // Directory or file name (not full path)
	Path     string         // Absolute path (for directories) or doc.Path (for files)
	RelPath  string         // Relative path from contextDir
	Doc      *Document      // Non-nil for leaf nodes (files)
	Children []*DocTreeNode // Non-nil for directory nodes
	Expanded bool           // Whether directory is expanded
}

// DocFlatNode is a flattened tree node for rendering.
type DocFlatNode struct {
	Node   *DocTreeNode
	Depth  int
	IsLast bool // last child of parent
}

// buildDocTree constructs a folder tree from a flat list of documents.
// Directories start expanded. Root-level files are direct children of the
// virtual root (the returned slice is the root level).
func buildDocTree(docs []Document) []*DocTreeNode {
	if len(docs) == 0 {
		return nil
	}

	// dirMap maps a relPath (directory) to its DocTreeNode.
	dirMap := map[string]*DocTreeNode{}
	// roots holds the top-level nodes (dirs and files at root level).
	var roots []*DocTreeNode

	// getOrCreateDir returns the DocTreeNode for a directory, creating
	// intermediate nodes as needed.
	var getOrCreateDir func(relDir string) *DocTreeNode
	getOrCreateDir = func(relDir string) *DocTreeNode {
		if node, ok := dirMap[relDir]; ok {
			return node
		}

		name := filepath.Base(relDir)
		node := &DocTreeNode{
			Name:     name,
			RelPath:  relDir,
			Children: []*DocTreeNode{},
			Expanded: true,
		}
		dirMap[relDir] = node

		parent := filepath.Dir(relDir)
		if parent == "." || parent == "" {
			// Top-level directory — attach to roots.
			roots = append(roots, node)
		} else {
			parentNode := getOrCreateDir(parent)
			parentNode.Children = append(parentNode.Children, node)
		}

		return node
	}

	for i := range docs {
		doc := &docs[i]
		parts := strings.Split(doc.RelPath, string(os.PathSeparator))

		leaf := &DocTreeNode{
			Name:    parts[len(parts)-1],
			Path:    doc.Path,
			RelPath: doc.RelPath,
			Doc:     doc,
		}

		if len(parts) == 1 {
			// Root-level file.
			roots = append(roots, leaf)
		} else {
			// File lives inside at least one directory.
			dirRelPath := filepath.Join(parts[:len(parts)-1]...)
			dirNode := getOrCreateDir(dirRelPath)
			dirNode.Children = append(dirNode.Children, leaf)
		}
	}

	sortDocTreeNodes(roots)
	return roots
}

// sortDocTreeNodes sorts nodes in-place: directories before files, then
// alphabetically within each group. Recurses into directory children.
func sortDocTreeNodes(nodes []*DocTreeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		iIsDir := nodes[i].Doc == nil
		jIsDir := nodes[j].Doc == nil
		if iIsDir != jIsDir {
			return iIsDir // directories first
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	for _, node := range nodes {
		if node.Doc == nil && len(node.Children) > 0 {
			sortDocTreeNodes(node.Children)
		}
	}
}

// flattenDocTree flattens expanded nodes into a renderable list.
func flattenDocTree(roots []*DocTreeNode) []DocFlatNode {
	var result []DocFlatNode

	var walk func(nodes []*DocTreeNode, depth int)
	walk = func(nodes []*DocTreeNode, depth int) {
		for i, node := range nodes {
			isLast := i == len(nodes)-1
			result = append(result, DocFlatNode{
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
