package review

import (
	"path/filepath"
	"testing"
)

// makeDoc is a helper that builds a Document with only Path and RelPath set.
func makeDoc(relPath string) Document {
	return Document{
		Path:    filepath.Join("/ctx", filepath.FromSlash(relPath)),
		RelPath: filepath.FromSlash(relPath),
	}
}

// TestBuildDocTree_Empty verifies that an empty input returns nil.
func TestBuildDocTree_Empty(t *testing.T) {
	roots := buildDocTree(nil)
	if roots != nil {
		t.Fatalf("expected nil roots for empty input, got %v", roots)
	}
}

// TestBuildDocTree_SingleRootFile verifies a single root-level file becomes one leaf node.
func TestBuildDocTree_SingleRootFile(t *testing.T) {
	docs := []Document{makeDoc("README.md")}
	roots := buildDocTree(docs)

	if len(roots) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(roots))
	}
	node := roots[0]
	if node.Doc == nil {
		t.Fatal("expected leaf node (Doc != nil)")
	}
	if node.Name != "README.md" {
		t.Errorf("expected Name=README.md, got %q", node.Name)
	}
	if len(node.Children) != 0 {
		t.Errorf("leaf node should have no children, got %d", len(node.Children))
	}
}

// TestBuildDocTree_FilesInSubdir verifies files in a subdirectory produce a directory node with children.
func TestBuildDocTree_FilesInSubdir(t *testing.T) {
	docs := []Document{
		makeDoc(filepath.Join("plans", "plan-a.md")),
		makeDoc(filepath.Join("plans", "plan-b.md")),
	}
	roots := buildDocTree(docs)

	if len(roots) != 1 {
		t.Fatalf("expected 1 root dir node, got %d", len(roots))
	}
	dir := roots[0]
	if dir.Doc != nil {
		t.Fatal("expected directory node (Doc == nil)")
	}
	if dir.Name != "plans" {
		t.Errorf("expected Name=plans, got %q", dir.Name)
	}
	if !dir.Expanded {
		t.Error("directory nodes should start expanded")
	}
	if len(dir.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(dir.Children))
	}
	for _, child := range dir.Children {
		if child.Doc == nil {
			t.Errorf("child %q should be a leaf node", child.Name)
		}
	}
}

// TestBuildDocTree_NestedDirs verifies nested directory nodes are created.
func TestBuildDocTree_NestedDirs(t *testing.T) {
	docs := []Document{
		makeDoc(filepath.Join("a", "b", "file.md")),
	}
	roots := buildDocTree(docs)

	if len(roots) != 1 {
		t.Fatalf("expected 1 root dir node, got %d", len(roots))
	}
	a := roots[0]
	if a.Name != "a" {
		t.Errorf("expected root Name=a, got %q", a.Name)
	}
	if len(a.Children) != 1 {
		t.Fatalf("expected 1 child of 'a', got %d", len(a.Children))
	}
	b := a.Children[0]
	if b.Name != "b" {
		t.Errorf("expected child Name=b, got %q", b.Name)
	}
	if len(b.Children) != 1 {
		t.Fatalf("expected 1 child of 'b', got %d", len(b.Children))
	}
	leaf := b.Children[0]
	if leaf.Doc == nil {
		t.Fatal("expected leaf node")
	}
	if leaf.Name != "file.md" {
		t.Errorf("expected leaf Name=file.md, got %q", leaf.Name)
	}
}

// TestBuildDocTree_MixedRootAndSubdir verifies root files and subdirectory files coexist.
func TestBuildDocTree_MixedRootAndSubdir(t *testing.T) {
	docs := []Document{
		makeDoc("root.md"),
		makeDoc(filepath.Join("plans", "plan.md")),
	}
	roots := buildDocTree(docs)

	// Expect 2 root-level nodes: one directory "plans", one file "root.md"
	if len(roots) != 2 {
		t.Fatalf("expected 2 root nodes, got %d", len(roots))
	}

	// Directories should come before files (sort order).
	if roots[0].Doc != nil {
		t.Error("first root node should be a directory (dirs before files)")
	}
	if roots[1].Doc == nil {
		t.Error("second root node should be a file")
	}
}

// TestBuildDocTree_SortOrder verifies directories appear before files and alphabetical order within each group.
func TestBuildDocTree_SortOrder(t *testing.T) {
	docs := []Document{
		makeDoc("z-file.md"),
		makeDoc(filepath.Join("beta", "b.md")),
		makeDoc("a-file.md"),
		makeDoc(filepath.Join("alpha", "a.md")),
	}
	roots := buildDocTree(docs)

	// Expected order: alpha/ (dir), beta/ (dir), a-file.md (file), z-file.md (file)
	if len(roots) != 4 {
		t.Fatalf("expected 4 root nodes, got %d", len(roots))
	}
	names := make([]string, len(roots))
	for i, r := range roots {
		names[i] = r.Name
	}

	expected := []string{"alpha", "beta", "a-file.md", "z-file.md"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("roots[%d]: expected %q, got %q", i, want, names[i])
		}
	}
}

// --- flattenDocTree tests ---

// TestFlattenDocTree_Empty verifies an empty tree returns nil.
func TestFlattenDocTree_Empty(t *testing.T) {
	result := flattenDocTree(nil)
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %d nodes", len(result))
	}
}

// TestFlattenDocTree_SingleNode verifies a single node is flattened correctly.
func TestFlattenDocTree_SingleNode(t *testing.T) {
	doc := makeDoc("readme.md")
	roots := []*DocTreeNode{
		{Name: "readme.md", Doc: &doc},
	}
	flat := flattenDocTree(roots)

	if len(flat) != 1 {
		t.Fatalf("expected 1 flat node, got %d", len(flat))
	}
	if flat[0].Depth != 0 {
		t.Errorf("expected depth 0, got %d", flat[0].Depth)
	}
	if !flat[0].IsLast {
		t.Error("single node should be IsLast")
	}
}

// TestFlattenDocTree_ExpandedDirectory verifies expanded directory includes its children.
func TestFlattenDocTree_ExpandedDirectory(t *testing.T) {
	docA := makeDoc(filepath.Join("plans", "a.md"))
	docB := makeDoc(filepath.Join("plans", "b.md"))

	childA := &DocTreeNode{Name: "a.md", Doc: &docA}
	childB := &DocTreeNode{Name: "b.md", Doc: &docB}
	dir := &DocTreeNode{
		Name:     "plans",
		Children: []*DocTreeNode{childA, childB},
		Expanded: true,
	}

	flat := flattenDocTree([]*DocTreeNode{dir})

	if len(flat) != 3 {
		t.Fatalf("expected 3 flat nodes (dir + 2 children), got %d", len(flat))
	}
	if flat[0].Depth != 0 {
		t.Errorf("dir depth should be 0, got %d", flat[0].Depth)
	}
	if flat[1].Depth != 1 {
		t.Errorf("child depth should be 1, got %d", flat[1].Depth)
	}
	// flat[1] = childA (IsLast=false), flat[2] = childB (IsLast=true)
	if flat[2].IsLast != true {
		t.Error("last child should have IsLast=true")
	}
	if flat[1].IsLast != false {
		t.Error("first child should have IsLast=false")
	}
}

// TestFlattenDocTree_CollapsedDirectory verifies collapsed directory hides its children.
func TestFlattenDocTree_CollapsedDirectory(t *testing.T) {
	docA := makeDoc(filepath.Join("plans", "a.md"))
	childA := &DocTreeNode{Name: "a.md", Doc: &docA}
	dir := &DocTreeNode{
		Name:     "plans",
		Children: []*DocTreeNode{childA},
		Expanded: false,
	}

	flat := flattenDocTree([]*DocTreeNode{dir})

	if len(flat) != 1 {
		t.Fatalf("expected 1 flat node (only dir, children hidden), got %d", len(flat))
	}
	if flat[0].Node.Name != "plans" {
		t.Errorf("expected plans node, got %q", flat[0].Node.Name)
	}
}

// TestFlattenDocTree_NestedDirs verifies depth increases correctly for nested directories.
func TestFlattenDocTree_NestedDirs(t *testing.T) {
	doc := makeDoc(filepath.Join("a", "b", "file.md"))
	leaf := &DocTreeNode{Name: "file.md", Doc: &doc}
	b := &DocTreeNode{Name: "b", Children: []*DocTreeNode{leaf}, Expanded: true}
	a := &DocTreeNode{Name: "a", Children: []*DocTreeNode{b}, Expanded: true}

	flat := flattenDocTree([]*DocTreeNode{a})

	if len(flat) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(flat))
	}
	depths := []int{0, 1, 2}
	for i, fn := range flat {
		if fn.Depth != depths[i] {
			t.Errorf("flat[%d] depth: expected %d, got %d", i, depths[i], fn.Depth)
		}
	}
}
