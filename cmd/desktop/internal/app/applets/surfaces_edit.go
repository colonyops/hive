package applets

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// Names of the keys under surfaces: in applet.yaml.
const (
	SurfaceTab      = "tab"
	SurfacePopup    = "popup"
	SurfaceTitlebar = "titlebar"
)

// SetSurface returns raw with one surface turned on or off, keeping the file's comments.
// Off is enabled: false rather than a removed key, so a pop-up keeps its size and a title
// bar keeps its page for when the surface comes back.
func SetSurface(raw []byte, surface string, on bool) ([]byte, error) {
	if !slices.Contains([]string{SurfaceTab, SurfacePopup, SurfaceTitlebar}, surface) {
		return nil, fmt.Errorf("no surface %q", surface)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("this file is not a map of keys")
	}
	surfaces, err := editableMapping(doc.Content[0], "surfaces")
	if err != nil {
		return nil, err
	}
	// A file that lists nothing is a pop-up. That has to be said out loud before another
	// surface joins the list, or the pop-up would go away.
	if len(surfaces.Content) == 0 {
		setMapValue(surfaces, SurfacePopup, emptyMapping())
	}
	if mapValue(surfaces, surface) == nil {
		if surface == SurfaceTitlebar {
			return nil, errors.New("this file has no title bar page. Add surfaces.titlebar with its entry")
		}
		setMapValue(surfaces, surface, emptyMapping())
	}
	settings, err := editableMapping(surfaces, surface)
	if err != nil {
		return nil, err
	}
	deleteMapValue(settings, "enabled")
	if !on {
		setMapValue(settings, "enabled", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"})
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// editableMapping finds or adds a map under key. A bare key becomes one. An alias stays
// refused, because an edit there would also change whatever else shares the anchor.
func editableMapping(parent *yaml.Node, key string) (*yaml.Node, error) {
	value := mapValue(parent, key)
	switch {
	case value == nil:
		value = emptyMapping()
		setMapValue(parent, key, value)
	case value.Kind == yaml.ScalarNode && value.ShortTag() == "!!null":
		*value = *emptyMapping()
	case value.Kind != yaml.MappingNode:
		return nil, fmt.Errorf("%s is not a plain map in this file, so edit it by hand", key)
	}
	return value, nil
}

// emptyMapping is flow style so that it writes as {}. A key added later stays on that line.
func emptyMapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Style: yaml.FlowStyle}
}

func mapValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setMapValue(mapping *yaml.Node, key string, value *yaml.Node) {
	name := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	mapping.Content = append(mapping.Content, name, value)
}

func deleteMapValue(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = slices.Delete(mapping.Content, i, i+2)
			return
		}
	}
}
