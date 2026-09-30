package actions

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// populateNonZero walks v (the struct behind a fresh ActionConfig, e.g. the
// *ShellConfig a registry factory returns) via reflection and assigns a
// deterministic non-zero value to every settable field.
//
// A hardcoded per-type fixture would need a hand-maintained literal for every
// field of every action type, and a forgotten field would round-trip a zero
// value on both sides without ever exercising the code path that drops it.
// Reflection instead touches every field a config struct has today or gains
// later, so TestActionTypesRoundTripThroughTheYAMLWriterAndLoader keeps
// covering a type's full schema with no fixture to fall out of sync.
func populateNonZero(t *testing.T, v reflect.Value, seed *int) {
	t.Helper()
	if !v.IsValid() || !v.CanSet() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		populateNonZero(t, v.Elem(), seed)
	case reflect.Struct:
		for _, field := range v.Fields() {
			populateNonZero(t, field, seed)
		}
	case reflect.String:
		*seed++
		v.SetString(fmt.Sprintf("value-%d", *seed))
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		*seed++
		v.SetInt(int64(*seed))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		*seed++
		v.SetUint(uint64(*seed))
	case reflect.Float32, reflect.Float64:
		*seed++
		v.SetFloat(float64(*seed))
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
		key := reflect.New(v.Type().Key()).Elem()
		populateNonZero(t, key, seed)
		val := reflect.New(v.Type().Elem()).Elem()
		populateNonZero(t, val, seed)
		v.SetMapIndex(key, val)
	case reflect.Slice:
		elem := reflect.New(v.Type().Elem()).Elem()
		populateNonZero(t, elem, seed)
		v.Set(reflect.Append(v, elem))
	default:
		t.Fatalf("populateNonZero: unsupported field kind %s; extend this helper for the new action config field", v.Kind())
	}
}

// TestActionTypesRoundTripThroughTheYAMLWriterAndLoader ranges every
// registered action type via Types() instead of a hardcoded list, so a new
// type is covered the moment its registry line lands (actions.go). For each
// type it builds a config from the registry's own factory, fills every field
// with a deterministic non-zero value via reflection, and drives it through
// the real save path: actionNode (the YAML writer in store.go) via
// ActionStore.Create, then LoadActions (via the store's reload after
// writing) reading it back.
//
// It fails if a registered type has no case in actionNode's switch — the
// store.go bug this test exists to catch — and it fails if any per-type
// field is dropped on the way to disk or back. It also confirms the
// editable-catalog branch (editableFromAction) does not silently drop the
// type either.
func TestActionTypesRoundTripThroughTheYAMLWriterAndLoader(t *testing.T) {
	for _, actionType := range Types() {
		t.Run(actionType, func(t *testing.T) {
			factory, ok := registry[actionType]
			require.Truef(t, ok, "type %q is in Types() but missing from registry", actionType)

			cfg := factory()
			seed := 0
			populateNonZero(t, reflect.ValueOf(cfg).Elem(), &seed)
			if launch, ok := cfg.(*LaunchSessionConfig); ok {
				// This schema-wide pass exercises the workspace branch; focused
				// tests cover the mutually exclusive repository and post-hook fields.
				launch.RepoTemplate = ""
				launch.Agent = ""
				launch.PostHook = ""
				launch.PostHookTimeout = 0
			}
			require.NoErrorf(t, cfg.Validate(), "populated %q config must satisfy its own Validate", actionType)

			original := Action{
				ID:           actionType,
				Label:        "Label for " + actionType,
				Type:         actionType,
				ShowInDetail: true,
				AppliesTo:    []string{"pr"},
				Config:       cfg,
			}

			s := NewActionStore(filepath.Join(t.TempDir(), "actions.yml"))
			s.mu.Lock()
			_, err := s.mutateLocked("create", original.ID, original)
			s.mu.Unlock()
			require.NoErrorf(t, err, "type %q: actionNode/the YAML save path rejected a validly-populated config", actionType)

			reloaded, ok := s.Get(actionType)
			require.Truef(t, ok, "type %q: not found after the writer/loader round trip", actionType)
			assert.Equalf(t, original.Config, reloaded.Config, "type %q: a per-type field was dropped on the writer/loader round trip", actionType)

			_, err = editableFromAction(original)
			require.NoErrorf(t, err, "type %q: editableFromAction has no editable-catalog branch for this type", actionType)
		})
	}
}

// fakeUnregisteredConfig is deliberately absent from actionNode's switch and
// editableFromAction's switch. It exists only to pin the guard: a config
// type the writer/editable-catalog does not know must fail loudly rather
// than silently produce a truncated action or an empty editable record.
type fakeUnregisteredConfig struct{}

func (fakeUnregisteredConfig) Validate() error { return nil }

func TestActionNodeFailsClosedOnAConfigTypeItDoesNotKnow(t *testing.T) {
	_, err := actionNode(Action{ID: "mystery", Type: "mystery", Config: fakeUnregisteredConfig{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mystery")
}

func TestEditableFromActionFailsClosedOnAConfigTypeItDoesNotKnow(t *testing.T) {
	_, err := editableFromAction(Action{ID: "mystery", Type: "mystery", Config: fakeUnregisteredConfig{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mystery")
}
