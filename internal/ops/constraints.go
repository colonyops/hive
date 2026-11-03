package ops

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// Constraint narrows one input field beyond what its Go type says. The MCP
// schema states it, and the CLI checks it before it calls.
type Constraint struct {
	Enum     []string
	Min, Max *float64
	// Default is the value an omitted field takes. It is documentation: the
	// handler must treat the zero value the same way.
	Default any
}

// Constraints is keyed by json field name.
type Constraints map[string]Constraint

// Range is an inclusive numeric bound.
func Range(lo, hi float64) Constraint { return Constraint{Min: &lo, Max: &hi} }

// Field is one input field with its constraint applied.
type Field struct {
	Name string
	// Type is the field's Go type with any pointer removed. Pointer says
	// whether there was one: an omitted pointer field keeps a stored value.
	Type        reflect.Type
	Pointer     bool
	Description string
	Required    bool
	Constraint
}

// Kind is the kind of Type.
func (f Field) Kind() reflect.Kind { return f.Type.Kind() }

// Fields lists the input fields of op in declaration order. A Fields key that
// names no field is an error.
func Fields(op Op) ([]Field, error) {
	return fields(op.InputType(), op.Info().Fields)
}

func fields(t reflect.Type, constraints Constraints) ([]Field, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("input %v is not a struct", t)
	}
	var out []Field
	for sf := range t.Fields() {
		name, opts, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if name == "-" || !sf.IsExported() || sf.Anonymous {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		ft, pointer := sf.Type, false
		if ft.Kind() == reflect.Pointer {
			ft, pointer = ft.Elem(), true
		}
		optional := slices.Contains(strings.Split(opts, ","), "omitempty") ||
			slices.Contains(strings.Split(opts, ","), "omitzero")
		out = append(out, Field{
			Name: name, Type: ft, Pointer: pointer, Description: sf.Tag.Get("jsonschema"),
			Required: !optional && !pointer, Constraint: constraints[name],
		})
	}
	for key := range constraints {
		if !slices.ContainsFunc(out, func(f Field) bool { return f.Name == key }) {
			return nil, fmt.Errorf("constraint %q names no field of %v", key, t)
		}
	}
	return out, nil
}

func inputSchema(t reflect.Type, constraints Constraints) (*jsonschema.Schema, error) {
	schema, err := jsonschema.ForType(t, &jsonschema.ForOptions{})
	if err != nil {
		return nil, err
	}
	list, err := fields(t, constraints)
	if err != nil {
		return nil, err
	}
	for _, f := range list {
		c := f.Constraint
		if len(c.Enum) == 0 && c.Min == nil && c.Max == nil && c.Default == nil {
			continue
		}
		prop := schema.Properties[f.Name]
		if prop == nil {
			return nil, fmt.Errorf("field %s has no schema property", f.Name)
		}
		for _, v := range c.Enum {
			prop.Enum = append(prop.Enum, v)
		}
		prop.Minimum, prop.Maximum = c.Min, c.Max
		if c.Default != nil {
			raw, err := json.Marshal(c.Default)
			if err != nil {
				return nil, fmt.Errorf("field %s default: %w", f.Name, err)
			}
			prop.Default = raw
		}
	}
	return schema, nil
}

// Check reports whether v satisfies the field's enum and range.
func (f Field) Check(v any) error {
	if s, ok := v.(string); ok && len(f.Enum) > 0 && !slices.Contains(f.Enum, s) {
		return Errorf(KindInvalid, "%s %q is not one of %s", f.Name, s, strings.Join(f.Enum, ", "))
	}
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case int:
		n = float64(x)
	case int64:
		n = float64(x)
	default:
		return nil
	}
	if f.Min != nil && n < *f.Min || f.Max != nil && n > *f.Max {
		return Errorf(KindInvalid, "%s %v is outside %v to %v", f.Name, n, bound(f.Min), bound(f.Max))
	}
	return nil
}

func bound(p *float64) any {
	if p == nil {
		return "any"
	}
	return *p
}
