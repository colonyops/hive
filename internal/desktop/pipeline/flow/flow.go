// Package flow implements the flows/*.yaml config schema for the desktop
// pipeline's Node-RED-style graph: a strict decoder, a per-node-type
// registry, cross-file reference validation, and directory loaders.
//
// A flow is a directed graph of Node values connected by Wire edges. The
// flow's id is not stored in the file — it is the filename stem (e.g.
// "triage.yaml" -> id "triage") so the file and its id can never disagree.
//
// This package is deliberately self-contained: it does not know about Wails,
// the desktop pipeline database, or internal/desktop/feed. Cross-file lookups
// (does a referenced source/feed/action exist, and what kind is it) are
// supplied by the caller through the Refs interface, so profiles/actions
// loaders can be wired in later without this package depending on them.
package flow

// Flow is one parsed and validated flows/*.yaml document.
type Flow struct {
	// ID is the filename stem (no extension), never a value read from the
	// file itself.
	ID      string
	Name    string
	Enabled bool
	Nodes   []Node
	Wires   []Wire
}

// Wire is a directed edge from one node's output port to another node's
// (sole) input. Out defaults to 0 when omitted in YAML.
type Wire struct {
	From string `yaml:"from"`
	Out  int    `yaml:"out,omitempty"`
	To   string `yaml:"to"`
}

// flowFile is the top-level on-disk shape of a flows/*.yaml document.
// Enabled is a pointer so an absent key can be distinguished from an
// explicit `enabled: false` and defaulted to true.
type flowFile struct {
	Version int    `yaml:"version"`
	Name    string `yaml:"name,omitempty"`
	Enabled *bool  `yaml:"enabled,omitempty"`
	Nodes   []Node `yaml:"nodes"`
	Wires   []Wire `yaml:"wires,omitempty"`
}
