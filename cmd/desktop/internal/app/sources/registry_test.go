package sources

import (
	"encoding/json"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/colonyops/hive/cmd/desktop/internal/app/credentials"
	"github.com/colonyops/hive/cmd/desktop/internal/app/sources/connector"
)

// A descriptor is the whole declaration of a connector: everything the editor,
// the docs, and later an MCP tool listing read comes from it. A field left
// zero is not a smaller connector, it is one that renders as a blank palette
// entry or an unroutable node type — so every one is required here rather
// than defaulted somewhere downstream.
func TestEveryDescriptorIsComplete(t *testing.T) {
	t.Parallel()

	for _, connectorType := range Types() {
		descriptor, ok := Lookup(connectorType)
		require.Truef(t, ok, "Types() returned %q but Lookup does not know it", connectorType)

		assert.Equalf(t, connectorType, descriptor.Type,
			"connector %q is registered under a key that is not its own Type", connectorType)
		assert.NotEmptyf(t, descriptor.Title, "connector %q has no title", connectorType)
		assert.Containsf(t, []connector.Mode{connector.ModePull, connector.ModePush}, descriptor.Mode,
			"connector %q declares no ingestion mode", connectorType)
		assert.NotZerof(t, descriptor.Stability, "connector %q declares no stability", connectorType)
		require.NotNilf(t, descriptor.NewConfig, "connector %q has no config factory", connectorType)
	}
}

// A descriptor's Provider is half of a credentials.Ref, so it has to satisfy
// the same rule a ref does. The mistake this catches is pasting the connector
// type in ("sources.github" instead of "github"): a ref built from it fails to
// parse, every source node on that connector reports a bad credential, and the
// Integrations card lists no accounts — three confusing symptoms of one typo.
func TestDescriptorProviderIsAValidCredentialProvider(t *testing.T) {
	t.Parallel()

	for _, connectorType := range Types() {
		descriptor, _ := Lookup(connectorType)
		if descriptor.Provider == "" {
			continue // a connector with nothing to authenticate as
		}
		ref := credentials.Ref{Provider: descriptor.Provider, Account: "account"}
		assert.NoErrorf(t, ref.Validate(),
			"connector %q declares provider %q, which is not a usable credentials provider",
			connectorType, descriptor.Provider)
	}
}

// The type string is the node's `type:` discriminator, the registry key, the
// docs filename, and the frontend's registry key. Namespacing it is what
// makes source connectors sort together everywhere those are enumerated, and
// it only holds if every connector follows it.
func TestConnectorTypesAreNamespaced(t *testing.T) {
	t.Parallel()

	for _, connectorType := range Types() {
		assert.Truef(t, strings.HasPrefix(connectorType, "sources."),
			"connector %q must be namespaced \"sources.<name>\"", connectorType)
		assert.NotEmptyf(t, strings.TrimPrefix(connectorType, "sources."),
			"connector %q has an empty name after its namespace", connectorType)
	}
}

// The decoder mutates a config in place, so a factory that returns a shared
// pointer would have two source nodes editing one value — the same constraint
// flow's node factories carry, restated here because a connector author only
// reads this package.
func TestNewConfigReturnsADistinctValue(t *testing.T) {
	t.Parallel()

	for _, connectorType := range Types() {
		descriptor, _ := Lookup(connectorType)
		first, second := descriptor.NewConfig(), descriptor.NewConfig()
		assert.NotSamef(t, first, second,
			"connector %q returns the same config value twice; the decoder mutates it in place", connectorType)
	}
}

// A zero config must fail validation. Every connector has at least one field
// it cannot work without (a kind, a path), and a config that validates empty
// means a source node saved with nothing filled in reaches the producer.
func TestZeroConfigDoesNotValidate(t *testing.T) {
	t.Parallel()

	for _, connectorType := range Types() {
		descriptor, _ := Lookup(connectorType)
		assert.Errorf(t, descriptor.NewConfig().Validate(),
			"connector %q accepts an empty config", connectorType)
	}
}

// The schema is what phase 7's MCP tool input schemas and a schema-rendered
// editor form are generated from. Reflecting it means it cannot describe a
// field the struct lacks, but it can still fail to produce an object schema
// at all — which fails silently as an empty form.
func TestEveryDescriptorReflectsAnObjectSchema(t *testing.T) {
	t.Parallel()

	for _, connectorType := range Types() {
		descriptor, _ := Lookup(connectorType)

		raw, err := connector.Schema(descriptor)
		require.NoErrorf(t, err, "connector %q", connectorType)

		var schema struct {
			Type       string                     `json:"type"`
			Title      string                     `json:"title"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		require.NoErrorf(t, json.Unmarshal(raw, &schema), "connector %q schema is not JSON", connectorType)

		assert.Equalf(t, "object", schema.Type, "connector %q schema is not an object", connectorType)
		assert.Equalf(t, descriptor.Title, schema.Title, "connector %q schema title", connectorType)
		assert.NotEmptyf(t, schema.Properties, "connector %q schema declares no properties", connectorType)
	}
}

// All returns a copy so a caller ranging over the registry cannot delete a
// connector from it. The registry is package state read by the flow and
// runtime node registries at init, so a mutation would be unrecoverable.
func TestAllReturnsACopy(t *testing.T) {
	t.Parallel()

	before := len(All())
	require.NotZero(t, before)

	mutated := All()
	for name := range mutated {
		delete(mutated, name)
	}

	assert.Len(t, All(), before)
}

// schemaProperty is the slice of a config's reflected JSON Schema this test
// needs: enough to fill a required field with a schema-honest placeholder
// without knowing the connector's Go type. See connector.Schema.
type schemaProperty struct {
	Type string   `json:"type"`
	Enum []any    `json:"enum"`
	Min  *float64 `json:"minimum"`
}

type schemaDoc struct {
	Required   []string                  `json:"required"`
	Properties map[string]schemaProperty `json:"properties"`
}

// structFieldByTagName finds v's field tagged json:"name" or, failing that,
// yaml:"name" — the two tag kinds a connector config declares its wire name
// under (see connector.Schema and the YAML decoder). v must be the
// addressable struct obtained from reflect.ValueOf(cfg).Elem().
func structFieldByTagName(v reflect.Value, name string) (reflect.Value, bool) {
	t := v.Type()
	for _, tagKey := range []string{"json", "yaml"} {
		for i := 0; i < t.NumField(); i++ {
			tag := t.Field(i).Tag.Get(tagKey)
			if tag == "" || tag == "-" {
				continue
			}
			if strings.Split(tag, ",")[0] == name {
				return v.Field(i), true
			}
		}
	}
	return reflect.Value{}, false
}

// fieldCandidates lists the values worth trying for one required schema
// property, purely from what the schema itself says: every enum option (a
// connector's "kind" field, say), or a single schema-shaped placeholder for a
// plain string/number/bool. A property this cannot characterize (an object,
// an array) yields nothing, and is left unset.
func fieldCandidates(prop schemaProperty) []any {
	if len(prop.Enum) > 0 {
		return prop.Enum
	}
	switch prop.Type {
	case "string":
		return []any{"test-value"}
	case "integer", "number":
		v := 1.0
		if prop.Min != nil {
			v = *prop.Min
		}
		return []any{v}
	case "boolean":
		return []any{true}
	default:
		return nil
	}
}

// setFieldValue assigns a schema-derived candidate (a string, float64, or
// bool — the shapes json.Unmarshal produces for a schema's scalar values)
// onto a struct field, converting to the field's own kind. It reports whether
// the assignment was possible.
func setFieldValue(field reflect.Value, value any) bool {
	if !field.CanSet() {
		return false
	}
	switch v := value.(type) {
	case string:
		if field.Kind() != reflect.String {
			return false
		}
		field.SetString(v)
	case float64:
		switch field.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			field.SetInt(int64(v))
		case reflect.Float32, reflect.Float64:
			field.SetFloat(v)
		default:
			return false
		}
	case bool:
		if field.Kind() != reflect.Bool {
			return false
		}
		field.SetBool(v)
	default:
		return false
	}
	return true
}

// fieldCombinations enumerates every combination of candidate values for the
// given required field names, crossed against each other. In practice a
// connector declares at most one enum-required field, so this is small, but
// nothing here assumes that — an unfillable field (fieldCandidates returns
// none) is simply left out of every combination, which is the honest
// approximation the field's absence deserves.
func fieldCombinations(doc schemaDoc, names []string) []map[string]any {
	combos := []map[string]any{{}}
	for _, name := range names {
		candidates := fieldCandidates(doc.Properties[name])
		if len(candidates) == 0 {
			continue
		}
		next := make([]map[string]any, 0, len(combos)*len(candidates))
		for _, combo := range combos {
			for _, value := range candidates {
				merged := make(map[string]any, len(combo)+1)
				maps.Copy(merged, combo)
				merged[name] = value
				next = append(next, merged)
			}
		}
		combos = next
	}
	return combos
}

// buildConfig constructs a fresh config for descriptor, sets its credential
// field (discovered by tag, never by connector-specific knowledge) to
// credential, applies the rest of combo the same way, and returns it already
// validated.
func buildConfig(t *testing.T, descriptor connector.Descriptor, credential string, combo map[string]any) (connector.Config, error) {
	t.Helper()

	cfg := descriptor.NewConfig()
	v := reflect.ValueOf(cfg)
	require.Equalf(t, reflect.Pointer, v.Kind(), "connector %q: NewConfig() must return a pointer for Validate to see the fields this test sets", descriptor.Type)
	v = v.Elem()

	credField, ok := structFieldByTagName(v, "credential")
	require.Truef(t, ok, "connector %q declares a Provider but its config has no field tagged credential", descriptor.Type)
	credField.SetString(credential)

	for name, value := range combo {
		if name == "credential" {
			continue
		}
		if field, ok := structFieldByTagName(v, name); ok {
			setFieldValue(field, value)
		}
	}

	return cfg, cfg.Validate()
}

// TestConfigValidateEnforcesDescriptorProvider ties a descriptor's Provider
// string to what its config actually enforces at Validate: a credential
// naming a different provider must be rejected, not silently accepted and
// used to fetch as the wrong account. Without this, a descriptor's Provider
// is a comment nobody checks — the connector's own Validate could ignore the
// credential's provider half entirely and nothing would fail.
//
// Discovery is generic on purpose: the credential field and every other
// schema-required field are found by tag name and JSON Schema shape (see
// connector.Schema), not by a per-connector type switch, so a connector added
// later is covered the same way github is today.
//
// The limitation this approach cannot paper over: Validate may enforce a
// cross-field rule the schema has no way to express (github's "kind: search
// requires a non-empty query", for instance — query has no omitempty-driven
// "required" in the schema because it is only conditionally required). When
// no schema-only combination of the other required fields makes Validate
// accept a correctly-provided credential, this logs that and falls back to
// checking only the half it can always check generically: that a
// provider-mismatched credential is rejected.
func TestConfigValidateEnforcesDescriptorProvider(t *testing.T) {
	t.Parallel()

	for _, connectorType := range Types() {
		descriptor, _ := Lookup(connectorType)
		if descriptor.Provider == "" {
			continue // webhook: local ingress, nothing to authenticate as, no provider to enforce
		}

		raw, err := connector.Schema(descriptor)
		require.NoErrorf(t, err, "connector %q", connectorType)
		var doc schemaDoc
		require.NoErrorf(t, json.Unmarshal(raw, &doc), "connector %q", connectorType)

		var otherRequired []string
		for _, name := range doc.Required {
			if name != "credential" {
				otherRequired = append(otherRequired, name)
			}
		}
		combos := fieldCombinations(doc, otherRequired)

		good := descriptor.Provider + "/test-account"
		bad := "not-" + descriptor.Provider + "/test-account"

		var accepting map[string]any
		found := false
		for _, combo := range combos {
			if _, err := buildConfig(t, descriptor, good, combo); err == nil {
				accepting = combo
				found = true
				break
			}
		}

		if !found {
			t.Logf("connector %q: no combination of its other schema-required fields makes Validate accept a "+
				"correctly-provided %q credential (a cross-field rule the schema cannot express); "+
				"asserting only that a mismatched provider is rejected", connectorType, descriptor.Provider)
			if len(combos) == 0 {
				combos = []map[string]any{{}}
			}
			accepting = combos[0]
		} else {
			_, err := buildConfig(t, descriptor, good, accepting)
			require.NoErrorf(t, err, "connector %q: a %q-provider credential should be accepted", connectorType, descriptor.Provider)
		}

		_, err = buildConfig(t, descriptor, bad, accepting)
		assert.Errorf(t, err, "connector %q: a credential naming a different provider than %q should be rejected", connectorType, descriptor.Provider)
	}
}
