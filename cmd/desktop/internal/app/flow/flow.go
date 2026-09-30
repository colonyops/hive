// Package flow implements the flows/*.yaml config schema for the desktop
// pipeline's Node-RED-style graph: a strict decoder, a per-node-type
// registry, cross-file reference validation, and directory loaders.
//
// A flow is a directed graph of Node values connected by Wire edges. The
// flow's id is not stored in the file — it is the filename stem (e.g.
// "triage.yaml" -> id "triage") so the file and its id can never disagree.
//
// This package is deliberately self-contained: it does not know about Wails,
// the desktop pipeline database, or internal/app/sources/github/feed. Cross-file action
// lookups are supplied by the caller through the Refs interface, so actions can
// stay owned by their own package without this package depending on them.
package flow

// Flow is one parsed and validated flows/*.yaml document. Besides being the
// decode target for LoadFlow, it is the wire shape GetFlow/SaveFlow expose
// to the desktop frontend's graph editor over Wails — hence the json tags
// alongside the (unused-by-Flow-itself, since flowFile is the YAML decode
// target) documentation of the on-disk names.
// ENUM(all, state-changes, never)
type ResurfacePolicy string

const DefaultResurfacePolicy = ResurfacePolicyStateChanges

type Flow struct {
	// ID is the filename stem (no extension), never a value read from the
	// file itself.
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Enabled   bool            `json:"enabled"`
	Resurface ResurfacePolicy `json:"resurface,omitempty"`
	// Image is the content hash of the profile's avatar, whose normalized PNG
	// lives in the app data dir (see internal/app/profileimg). It is owned by
	// SetImage, not the graph editor: FlowStore.Save preserves whatever is on
	// disk so a graph save never drops it. Empty means "no avatar" — the rail
	// falls back to the letter chip.
	Image string `json:"image,omitempty"`
	Nodes []Node `json:"nodes"`
	Wires []Wire `json:"wires"`
}

// FeedID is the id inbox membership uses for one of this flow's feed nodes:
// the node id qualified by the flow's, which is what makes it unique across
// profiles that reuse a node id.
func (f Flow) FeedID(nodeID string) string { return f.ID + "/" + nodeID }

// FeedNodes returns the flow's feed terminals in declaration order. A feed
// exists because a node declares it, not because anything has landed in it, so
// this is the authoritative list — inbox counts are a join onto it.
func (f Flow) FeedNodes() []Node {
	out := make([]Node, 0, len(f.Nodes))
	for _, n := range f.Nodes {
		if n.Type == NodeTypeFeed {
			out = append(out, n)
		}
	}
	return out
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
	Version   int             `yaml:"version"`
	Name      string          `yaml:"name,omitempty"`
	Enabled   *bool           `yaml:"enabled,omitempty"`
	Resurface ResurfacePolicy `yaml:"resurface,omitempty"`
	Image     string          `yaml:"image,omitempty"`
	Nodes     []Node          `yaml:"nodes"`
	Wires     []Wire          `yaml:"wires,omitempty"`
}
