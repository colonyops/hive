package flow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// SidebarFolder is one named, collapsible group of feeds in a profile's
// sidebar. Feeds lists the member feed node ids in display order.
type SidebarFolder struct {
	ID        string   `json:"id"        yaml:"id"`
	Name      string   `json:"name"      yaml:"name"`
	Collapsed bool     `json:"collapsed" yaml:"collapsed"`
	Feeds     []string `json:"feeds"     yaml:"feeds"`
}

// SidebarItem is one entry in the sidebar's ordered top level: either a bare
// feed (Feed set to a feed node id) or a folder (Folder set). Exactly one is
// meant to be non-empty; a malformed item with neither is ignored by the
// frontend when it reconciles the layout against the flow's actual feed nodes.
type SidebarItem struct {
	Feed   string         `json:"feed,omitempty"   yaml:"feed,omitempty"`
	Folder *SidebarFolder `json:"folder,omitempty" yaml:"folder,omitempty"`
}

// SidebarLayout is the machine-written, non-authoritative sibling of a flow —
// the on-disk shape of a <id>.sidebar.yaml file. It records how a profile's
// feed nodes are grouped into folders and ordered in the sidebar's FEEDS
// section. Like Layout (.ui.yaml) it is purely cosmetic and keyed by node id:
// a missing or broken file just means the sidebar falls back to listing feeds
// in flow-node order, feed nodes absent from the layout are appended (so a
// newly added feed is never hidden), and entries for feeds that no longer
// exist are dropped. It is never consulted by LoadFlow/validation and never
// blocks a flow from loading.
type SidebarLayout struct {
	Items []SidebarItem `json:"items" yaml:"items"`
}

// LoadSidebar reads a flow's sibling sidebar-layout file. A missing file, or
// one that fails to parse, is not an error — the layout is purely cosmetic —
// it returns an empty SidebarLayout instead so the caller always has an
// (empty) slice to range over.
func LoadSidebar(path string) SidebarLayout {
	data, err := os.ReadFile(path)
	if err != nil {
		return SidebarLayout{Items: []SidebarItem{}}
	}
	var layout SidebarLayout
	if err := yaml.Unmarshal(data, &layout); err != nil {
		return SidebarLayout{Items: []SidebarItem{}}
	}
	if layout.Items == nil {
		layout.Items = []SidebarItem{}
	}
	return layout
}

// SaveSidebar writes a flow's sidebar layout atomically. Like SaveUI it always
// marshals a clean document: sidebar-layout files are machine-written only and
// carry no hand-authored comments worth preserving.
func SaveSidebar(path string, layout SidebarLayout) error {
	if layout.Items == nil {
		layout.Items = []SidebarItem{}
	}
	data, err := yaml.Marshal(layout)
	if err != nil {
		return fmt.Errorf("flow: encode sidebar layout: %w", err)
	}
	return writeFileAtomic(path, data)
}
