package lua

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

// captureModule collects strings, errors, and arbitrary tagged values produced
// by Lua so the Go test side can assert on them. All access to the maps is
// through the dispatcher goroutine so no mutex is required, but we keep one
// for safety in case future tests share an instance across goroutines.
type captureModule struct {
	mu      sync.Mutex
	strings map[string]string
	errs    map[string]string
	bools   map[string]bool
	numbers map[string]float64
	any     map[string]any
}

func newCaptureModule() *captureModule {
	return &captureModule{
		strings: map[string]string{},
		errs:    map[string]string{},
		bools:   map[string]bool{},
		numbers: map[string]float64{},
		any:     map[string]any{},
	}
}

func (c *captureModule) Register(state *glua.LState, hive *glua.LTable) error {
	state.SetField(hive, "test_capture", state.NewFunction(func(state *glua.LState) int {
		key := state.CheckString(1)
		val := state.CheckString(2)
		c.mu.Lock()
		c.strings[key] = val
		c.mu.Unlock()
		return 0
	}))
	state.SetField(hive, "test_capture_err", state.NewFunction(func(state *glua.LState) int {
		key := state.CheckString(1)
		val := state.CheckString(2)
		c.mu.Lock()
		c.errs[key] = val
		c.mu.Unlock()
		return 0
	}))
	state.SetField(hive, "test_capture_bool", state.NewFunction(func(state *glua.LState) int {
		key := state.CheckString(1)
		val := state.CheckBool(2)
		c.mu.Lock()
		c.bools[key] = val
		c.mu.Unlock()
		return 0
	}))
	state.SetField(hive, "test_capture_number", state.NewFunction(func(state *glua.LState) int {
		key := state.CheckString(1)
		val := state.CheckNumber(2)
		c.mu.Lock()
		c.numbers[key] = float64(val)
		c.mu.Unlock()
		return 0
	}))
	return nil
}

// jsonHarness wires a Runtime + JSONModule + captureModule and runs the
// supplied Lua script as the entrypoint.
type jsonHarness struct {
	runtime *Runtime
	capture *captureModule
}

func newJSONHarness(t *testing.T, script string) *jsonHarness {
	t.Helper()

	root := t.TempDir()
	entry := filepath.Join(root, "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(script), 0o644))

	capture := newCaptureModule()
	rt, err := NewRuntime(
		root,
		zerolog.Nop(),
		&LogModule{PluginName: "lua-test", Logger: zerolog.Nop()},
		&PluginInfoModule{Name: "lua-test", Entry: entry, ModuleRoot: root},
		&JSONModule{},
		capture,
	)
	require.NoError(t, err)

	fn, err := rt.LoadEntrypoint(entry)
	require.NoError(t, err)
	require.NoError(t, rt.CallEntrypoint(fn))

	return &jsonHarness{runtime: rt, capture: capture}
}

func (h *jsonHarness) close(t *testing.T) {
	t.Helper()
	h.runtime.Close()
}

func TestJSONEncodePrimitives(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  hive.test_capture("nil",    hive.json.encode(nil))
  hive.test_capture("true",   hive.json.encode(true))
  hive.test_capture("false",  hive.json.encode(false))
  hive.test_capture("number", hive.json.encode(42))
  hive.test_capture("string", hive.json.encode("hello"))
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.Equal(t, "null", h.capture.strings["nil"])
	assert.Equal(t, "true", h.capture.strings["true"])
	assert.Equal(t, "false", h.capture.strings["false"])
	assert.Equal(t, "42", h.capture.strings["number"])
	assert.Equal(t, `"hello"`, h.capture.strings["string"])
}

func TestJSONEncodeArrayAndObject(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  hive.test_capture("array",  hive.json.encode({"a", "b", "c"}))
  hive.test_capture("object", hive.json.encode({ foo = 1 }))
end
`)
	t.Cleanup(func() { h.close(t) })

	var arr []any
	require.NoError(t, json.Unmarshal([]byte(h.capture.strings["array"]), &arr))
	assert.Equal(t, []any{"a", "b", "c"}, arr)

	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.capture.strings["object"]), &obj))
	assert.Equal(t, map[string]any{"foo": float64(1)}, obj)
}

func TestJSONEncodeNestedMixed(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  hive.test_capture("mixed", hive.json.encode({
    foo = 1,
    bar = { "a", "b" },
    baz = { x = true },
  }))
end
`)
	t.Cleanup(func() { h.close(t) })

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.capture.strings["mixed"]), &got))
	assert.Equal(t, map[string]any{
		"foo": float64(1),
		"bar": []any{"a", "b"},
		"baz": map[string]any{"x": true},
	}, got)
}

func TestJSONEncodeArrayMarker(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  hive.test_capture("empty_array",  hive.json.encode(hive.json.array({})))
  hive.test_capture("empty_object", hive.json.encode({}))
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.Equal(t, "[]", h.capture.strings["empty_array"])
	assert.Equal(t, "{}", h.capture.strings["empty_object"])
}

func TestJSONEncodePretty(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  hive.test_capture("pretty", hive.json.encode({ foo = 1 }, { pretty = true }))
end
`)
	t.Cleanup(func() { h.close(t) })

	out := h.capture.strings["pretty"]
	assert.Contains(t, out, "\n")
	assert.Contains(t, out, "  ")
	// Validate it still round-trips through encoding/json.
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, map[string]any{"foo": float64(1)}, got)
}

func TestJSONEncodeRejectsCycle(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local t = {}
  t.self = t
  local ok, err = pcall(hive.json.encode, t)
  if ok then
    hive.test_capture_err("cycle", "no-error")
  else
    hive.test_capture_err("cycle", tostring(err))
  end
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.Contains(t, h.capture.errs["cycle"], "cyclic")
}

func TestJSONEncodeRejectsFunction(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local ok, err = pcall(hive.json.encode, function() end)
  if ok then
    hive.test_capture_err("func", "no-error")
  else
    hive.test_capture_err("func", tostring(err))
  end
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.Contains(t, h.capture.errs["func"], "function")
}

func TestJSONDecodeObject(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local got = hive.json.decode('{"foo":1,"bar":"baz","ok":true}')
  hive.test_capture_number("foo", got.foo)
  hive.test_capture("bar", got.bar)
  hive.test_capture_bool("ok",  got.ok)
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.InDelta(t, 1.0, h.capture.numbers["foo"], 0)
	assert.Equal(t, "baz", h.capture.strings["bar"])
	assert.True(t, h.capture.bools["ok"])
}

func TestJSONDecodeArray(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local got = hive.json.decode('[10, 20, 30]')
  hive.test_capture_number("len",  #got)
  hive.test_capture_number("one",  got[1])
  hive.test_capture_number("two",  got[2])
  hive.test_capture_number("three", got[3])
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.InDelta(t, 3.0, h.capture.numbers["len"], 0)
	assert.InDelta(t, 10.0, h.capture.numbers["one"], 0)
	assert.InDelta(t, 20.0, h.capture.numbers["two"], 0)
	assert.InDelta(t, 30.0, h.capture.numbers["three"], 0)
}

func TestJSONDecodeMalformed(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local ok, err = pcall(hive.json.decode, '{')
  if ok then
    hive.test_capture_err("bad", "no-error")
  else
    hive.test_capture_err("bad", tostring(err))
  end
end
`)
	t.Cleanup(func() { h.close(t) })

	got := h.capture.errs["bad"]
	assert.Contains(t, got, "hive.json.decode")
	assert.Contains(t, strings.ToLower(got), "offset")
}

func TestJSONEmptyArrayRoundTrip(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local decoded = hive.json.decode('[]')
  hive.test_capture("re_encoded", hive.json.encode(decoded))
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.Equal(t, "[]", h.capture.strings["re_encoded"])
}

func TestJSONRoundTripNested(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local input = '{"foo":[1,2,3],"bar":{"baz":true}}'
  local decoded = hive.json.decode(input)
  hive.test_capture("re_encoded", hive.json.encode(decoded))
end
`)
	t.Cleanup(func() { h.close(t) })

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.capture.strings["re_encoded"]), &got))
	assert.Equal(t, map[string]any{
		"foo": []any{float64(1), float64(2), float64(3)},
		"bar": map[string]any{"baz": true},
	}, got)
}

func TestJSONEncodeArrayMarkerRejectsNonArrayKeys(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local string_key = hive.json.array({ foo = "x" })
  local ok1, err1 = pcall(hive.json.encode, string_key)
  hive.test_capture_err("string_key", tostring(err1))
  if ok1 then hive.test_capture_err("string_key_ok", "true") end

  local mixed = hive.json.array({ "a", "b", foo = "x" })
  local ok2, err2 = pcall(hive.json.encode, mixed)
  hive.test_capture_err("mixed", tostring(err2))
  if ok2 then hive.test_capture_err("mixed_ok", "true") end

  local sparse = hive.json.array({ [1] = "a", [3] = "c" })
  local ok3, err3 = pcall(hive.json.encode, sparse)
  hive.test_capture_err("sparse", tostring(err3))
  if ok3 then hive.test_capture_err("sparse_ok", "true") end
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.NotContains(t, h.capture.errs, "string_key_ok", "encode should reject array-tagged table with string key")
	assert.Contains(t, h.capture.errs["string_key"], "array-tagged")
	assert.Contains(t, h.capture.errs["string_key"], "non-integer key")

	assert.NotContains(t, h.capture.errs, "mixed_ok")
	assert.Contains(t, h.capture.errs["mixed"], "non-integer key")

	assert.NotContains(t, h.capture.errs, "sparse_ok")
	assert.Contains(t, h.capture.errs["sparse"], "hole at index 2")
}

func TestJSONEncodeRejectsUnsupportedKeyTypes(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local bool_key = {}
  bool_key[true] = "x"
  local ok1, err1 = pcall(hive.json.encode, bool_key)
  hive.test_capture_err("bool", tostring(err1))
  if ok1 then hive.test_capture_err("bool_ok", "true") end

  local table_key = {}
  table_key[{}] = "x"
  local ok2, err2 = pcall(hive.json.encode, table_key)
  hive.test_capture_err("table", tostring(err2))
  if ok2 then hive.test_capture_err("table_ok", "true") end
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.NotContains(t, h.capture.errs, "bool_ok")
	assert.Contains(t, h.capture.errs["bool"], "boolean as object key")

	assert.NotContains(t, h.capture.errs, "table_ok")
	assert.Contains(t, h.capture.errs["table"], "table as object key")
}

func TestJSONEncodeOptsValidation(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  local ok1, err1 = pcall(hive.json.encode, {}, { pretty = "true" })
  hive.test_capture_err("non_bool", tostring(err1))
  if ok1 then hive.test_capture_err("non_bool_ok", "true") end

  local ok2, err2 = pcall(hive.json.encode, {}, { prety = true })
  hive.test_capture_err("typo", tostring(err2))
  if ok2 then hive.test_capture_err("typo_ok", "true") end

  -- Valid opts still works.
  hive.test_capture("valid", hive.json.encode({ foo = 1 }, { pretty = true }))
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.NotContains(t, h.capture.errs, "non_bool_ok")
	assert.Contains(t, h.capture.errs["non_bool"], "opts.pretty must be a boolean")

	assert.NotContains(t, h.capture.errs, "typo_ok")
	assert.Contains(t, h.capture.errs["typo"], `unknown opts key "prety"`)

	assert.Contains(t, h.capture.strings["valid"], "\n")
}

func TestJSONDecodedArrayRoundTripsAfterMutation(t *testing.T) {
	t.Parallel()
	h := newJSONHarness(t, `
return function(hive)
  -- Remove the only element and re-encode: must stay [], not become {}.
  local arr = hive.json.decode('[1]')
  table.remove(arr, 1)
  hive.test_capture("after_drain", hive.json.encode(arr))

  -- Remove a middle element from a longer array; result is still 1..n.
  local arr2 = hive.json.decode('[1,2,3]')
  table.remove(arr2, 2)
  hive.test_capture("after_remove_middle", hive.json.encode(arr2))
end
`)
	t.Cleanup(func() { h.close(t) })

	assert.Equal(t, "[]", h.capture.strings["after_drain"])

	var arr []any
	require.NoError(t, json.Unmarshal([]byte(h.capture.strings["after_remove_middle"]), &arr))
	assert.Equal(t, []any{float64(1), float64(3)}, arr)
}

func TestJSONModuleRegistersInPlugin(t *testing.T) {
	t.Parallel()
	entry := filepath.Join(t.TempDir(), "init.lua")
	require.NoError(t, os.WriteFile(entry, []byte(`
return function(hive)
  if type(hive.json)        ~= "table"    then error("hive.json missing")        end
  if type(hive.json.encode) ~= "function" then error("hive.json.encode missing") end
  if type(hive.json.decode) ~= "function" then error("hive.json.decode missing") end
  if type(hive.json.array)  ~= "function" then error("hive.json.array missing")  end
end
`), 0o644))

	plugin := NewConfigPlugin(entry)
	require.NoError(t, plugin.Init(context.Background()))
	t.Cleanup(func() { require.NoError(t, plugin.Close()) })
}
