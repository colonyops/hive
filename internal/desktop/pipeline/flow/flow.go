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
// (does a referenced action exist, and what kind is it) are supplied by the
// caller through the Refs interface, so actions can stay owned by their own
// package without this package depending on them.
package flow

// Flow is one parsed and validated flows/*.yaml document. Besides being the
// decode target for LoadFlow, it is the wire shape GetFlow/SaveFlow expose
// to the desktop frontend's graph editor over Wails — hence the json tags
// alongside the (unused-by-Flow-itself, since flowFile is the YAML decode
// target) documentation of the on-disk names.
type Flow struct {
	// ID is the filename stem (no extension), never a value read from the
	// file itself.
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Nodes   []Node `json:"nodes"`
	Wires   []Wire `json:"wires"`
}

// Wire is a directed edge from one node's output port to another node's
// (sole) input. Out defaults to 0 when omitted in YAML.
type Wire struct {
	From string `json:"from"          yaml:"from"`
	Out  int    `json:"out,omitempty" yaml:"out,omitempty"`
	To   string `json:"to"            yaml:"to"`
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
