package connector

import (
	"encoding/json"
	"fmt"

	"github.com/invopop/jsonschema"
)

// Schema returns the JSON Schema of a connector's config, reflected from the
// struct's json and jsonschema tags.
//
// Reflecting rather than hand-writing it is the whole point: the schema an
// editor form, an MCP tool's input schema, and the docs are generated from
// cannot then describe a field the struct does not have. It is computed on
// demand rather than stored on the Descriptor so the descriptor stays plain
// data that a package-level map can hold.
func Schema(d Descriptor) (json.RawMessage, error) {
	if d.NewConfig == nil {
		return nil, fmt.Errorf("connector %q: no config factory", d.Type)
	}

	r := &jsonschema.Reflector{
		// The config is the whole schema, not a $ref into a $defs table: its
		// consumers (an editor form, an MCP tool's inputSchema) take one
		// object schema and have nowhere to resolve a reference from.
		ExpandedStruct: true,
		DoNotReference: true,
		// A field is required unless its json tag says omitempty, which is
		// already how the YAML round-trip distinguishes "unset" from "empty".
		RequiredFromJSONSchemaTags: false,
	}

	schema := r.Reflect(d.NewConfig())
	schema.Version = ""
	schema.Title = d.Title

	out, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("connector %q: encoding config schema: %w", d.Type, err)
	}
	return out, nil
}
