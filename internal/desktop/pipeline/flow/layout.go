package flow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// NodePosition is one node's canvas coordinates in a flow's sibling
// <id>.ui.yaml layout file.
type NodePosition struct {
	X int `json:"x" yaml:"x"`
	Y int `json:"y" yaml:"y"`
}

// Layout is the machine-written, non-authoritative sibling of a flow — and
// the on-disk shape of a <id>.ui.yaml file: node canvas positions only,
// keyed by node id. It is never consulted by LoadFlow/validation and never
// blocks a flow from loading — a missing or broken .ui.yaml just means the
// editor lays out nodes fresh (see LoadUI).
type Layout struct {
	Nodes map[string]NodePosition `json:"nodes" yaml:"nodes"`
}

// LoadUI reads a flow's sibling layout file. A missing file, or one that
// fails to parse, is not an error — Layout is purely cosmetic — it returns
// an empty Layout instead so the caller always has something to range over.
func LoadUI(path string) Layout {
	data, err := os.ReadFile(path)
	if err != nil {
		return Layout{Nodes: map[string]NodePosition{}}
	}
	var layout Layout
	if err := yaml.Unmarshal(data, &layout); err != nil {
		return Layout{Nodes: map[string]NodePosition{}}
	}
	if layout.Nodes == nil {
		layout.Nodes = map[string]NodePosition{}
	}
	return layout
}

// SaveUI writes a flow's layout atomically. Unlike SaveFlow, this always
// marshals a clean document: layout files are machine-written only and
// carry no hand-authored comments worth preserving.
func SaveUI(path string, layout Layout) error {
	if layout.Nodes == nil {
		layout.Nodes = map[string]NodePosition{}
	}
	data, err := yaml.Marshal(layout)
	if err != nil {
		return fmt.Errorf("flow: encode layout: %w", err)
	}
	return writeFileAtomic(path, data)
}
