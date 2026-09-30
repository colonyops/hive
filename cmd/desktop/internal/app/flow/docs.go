package flow

import (
	"embed"
	"fmt"
	"sort"
)

// docsFS holds one markdown file per registered node type, named
// docs/<type>.md. These files are the single source of truth for how a node
// type is explained to a human *and* to an LLM: the desktop node editor
// drawer and palette import them directly through the frontend's
// "@nodedocs" Vite alias (see desktop/frontend/vite.config.ts), and
// internal/app/prompts renders the same bytes into the flows authoring
// prompt. Documentation lives here, next to the schema that validates it,
// so the two cannot drift.
//
// Adding a node type means adding its registry entry and its docs/<type>.md;
// TestNodeDocsCoverEveryRegisteredType fails if either is missing.
//
//go:embed docs/*.md
var docsFS embed.FS

// NodeCategory groups node types the way the palette and the flows prompt
// present them. It is derived from a type's port counts rather than declared,
// so it cannot disagree with what the graph validator enforces.
type NodeCategory string

const (
	// CategorySources covers node types with no inputs — where a flow starts.
	CategorySources NodeCategory = "Sources"
	// CategoryProcess covers node types with both inputs and outputs.
	CategoryProcess NodeCategory = "Process"
	// CategoryDestinations covers terminal node types with no outputs.
	CategoryDestinations NodeCategory = "Destinations"
)

// CategoryOrder is the presentation order for the three categories: a flow
// reads source to destination, and so does its documentation.
var CategoryOrder = []NodeCategory{CategorySources, CategoryProcess, CategoryDestinations}

// NodeTypes returns every registered node type in sorted order. Sorting keeps
// prompt output byte-stable across runs — Go map iteration is randomized.
func NodeTypes() []string {
	types := make([]string, 0, len(registry))
	for nodeType := range registry {
		types = append(types, nodeType)
	}
	sort.Strings(types)
	return types
}

// NodeDoc returns the markdown documentation for a registered node type.
func NodeDoc(nodeType string) (string, error) {
	data, err := docsFS.ReadFile("docs/" + nodeType + ".md")
	if err != nil {
		return "", fmt.Errorf("flow: no documentation for node type %q", nodeType)
	}
	return string(data), nil
}

// CategoryOf classifies a registered node type from the port counts its
// config reports: no inputs makes it a source, no outputs a destination,
// anything else a processor.
func CategoryOf(nodeType string) (NodeCategory, error) {
	factory, ok := registry[nodeType]
	if !ok {
		return "", fmt.Errorf("flow: unknown node type %q", nodeType)
	}
	cfg := factory()
	switch {
	case cfg.Inputs() == 0:
		return CategorySources, nil
	case cfg.Outputs() == 0:
		return CategoryDestinations, nil
	default:
		return CategoryProcess, nil
	}
}
