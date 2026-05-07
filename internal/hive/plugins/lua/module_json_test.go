package lua

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

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
		&LogModule{PluginName: "lua-test"},
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
