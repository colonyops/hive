package lua

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONEncodePrimitives(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  hive.test_capture("nil",    hive.json.encode(nil))
  hive.test_capture("true",   hive.json.encode(true))
  hive.test_capture("false",  hive.json.encode(false))
  hive.test_capture("number", hive.json.encode(42))
  hive.test_capture("string", hive.json.encode("hello"))
end
`, &JSONModule{})

	assert.Equal(t, "null", h.capture.String("nil"))
	assert.Equal(t, "true", h.capture.String("true"))
	assert.Equal(t, "false", h.capture.String("false"))
	assert.Equal(t, "42", h.capture.String("number"))
	assert.Equal(t, `"hello"`, h.capture.String("string"))
}

func TestJSONEncodeArrayAndObject(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  hive.test_capture("array",  hive.json.encode({"a", "b", "c"}))
  hive.test_capture("object", hive.json.encode({ foo = 1 }))
end
`, &JSONModule{})

	var arr []any
	require.NoError(t, json.Unmarshal([]byte(h.capture.String("array")), &arr))
	assert.Equal(t, []any{"a", "b", "c"}, arr)

	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.capture.String("object")), &obj))
	assert.Equal(t, map[string]any{"foo": float64(1)}, obj)
}

func TestJSONEncodeNestedMixed(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  hive.test_capture("mixed", hive.json.encode({
    foo = 1,
    bar = { "a", "b" },
    baz = { x = true },
  }))
end
`, &JSONModule{})

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.capture.String("mixed")), &got))
	assert.Equal(t, map[string]any{
		"foo": float64(1),
		"bar": []any{"a", "b"},
		"baz": map[string]any{"x": true},
	}, got)
}

func TestJSONEncodeArrayMarker(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  hive.test_capture("empty_array",  hive.json.encode(hive.json.array({})))
  hive.test_capture("empty_object", hive.json.encode({}))
end
`, &JSONModule{})

	assert.Equal(t, "[]", h.capture.String("empty_array"))
	assert.Equal(t, "{}", h.capture.String("empty_object"))
}

func TestJSONEncodePretty(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  hive.test_capture("pretty", hive.json.encode({ foo = 1 }, { pretty = true }))
end
`, &JSONModule{})

	out := h.capture.String("pretty")
	assert.Contains(t, out, "\n")
	assert.Contains(t, out, "  ")
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, map[string]any{"foo": float64(1)}, got)
}

func TestJSONEncodeRejectsCycle(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local t = {}
  t.self = t
  local ok, err = pcall(hive.json.encode, t)
  if ok then
    hive.test_capture("cycle", "no-error")
  else
    hive.test_capture("cycle", tostring(err))
  end
end
`, &JSONModule{})

	assert.Contains(t, h.capture.String("cycle"), "cyclic")
}

func TestJSONEncodeRejectsFunction(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local ok, err = pcall(hive.json.encode, function() end)
  if ok then
    hive.test_capture("func", "no-error")
  else
    hive.test_capture("func", tostring(err))
  end
end
`, &JSONModule{})

	assert.Contains(t, h.capture.String("func"), "function")
}

func TestJSONDecodeObject(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local got = hive.json.decode('{"foo":1,"bar":"baz","ok":true}')
  hive.test_capture("foo", got.foo)
  hive.test_capture("bar", got.bar)
  hive.test_capture("ok",  got.ok)
end
`, &JSONModule{})

	assert.InDelta(t, 1.0, h.capture.Number("foo"), 0)
	assert.Equal(t, "baz", h.capture.String("bar"))
	assert.True(t, h.capture.Bool("ok"))
}

func TestJSONDecodeArray(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local got = hive.json.decode('[10, 20, 30]')
  hive.test_capture("len",   #got)
  hive.test_capture("one",   got[1])
  hive.test_capture("two",   got[2])
  hive.test_capture("three", got[3])
end
`, &JSONModule{})

	assert.InDelta(t, 3.0, h.capture.Number("len"), 0)
	assert.InDelta(t, 10.0, h.capture.Number("one"), 0)
	assert.InDelta(t, 20.0, h.capture.Number("two"), 0)
	assert.InDelta(t, 30.0, h.capture.Number("three"), 0)
}

func TestJSONDecodeMalformed(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local ok, err = pcall(hive.json.decode, '{')
  if ok then
    hive.test_capture("bad", "no-error")
  else
    hive.test_capture("bad", tostring(err))
  end
end
`, &JSONModule{})

	got := h.capture.String("bad")
	assert.Contains(t, got, "hive.json.decode")
	assert.Contains(t, strings.ToLower(got), "offset")
}

func TestJSONEmptyArrayRoundTrip(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local decoded = hive.json.decode('[]')
  hive.test_capture("re_encoded", hive.json.encode(decoded))
end
`, &JSONModule{})

	assert.Equal(t, "[]", h.capture.String("re_encoded"))
}

func TestJSONRoundTripNested(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local input = '{"foo":[1,2,3],"bar":{"baz":true}}'
  local decoded = hive.json.decode(input)
  hive.test_capture("re_encoded", hive.json.encode(decoded))
end
`, &JSONModule{})

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.capture.String("re_encoded")), &got))
	assert.Equal(t, map[string]any{
		"foo": []any{float64(1), float64(2), float64(3)},
		"bar": map[string]any{"baz": true},
	}, got)
}

func TestJSONEncodeArrayMarkerRejectsNonArrayKeys(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local string_key = hive.json.array({ foo = "x" })
  local ok1, err1 = pcall(hive.json.encode, string_key)
  hive.test_capture("string_key", tostring(err1))
  if ok1 then hive.test_capture("string_key_ok", "true") end

  local mixed = hive.json.array({ "a", "b", foo = "x" })
  local ok2, err2 = pcall(hive.json.encode, mixed)
  hive.test_capture("mixed", tostring(err2))
  if ok2 then hive.test_capture("mixed_ok", "true") end

  local sparse = hive.json.array({ [1] = "a", [3] = "c" })
  local ok3, err3 = pcall(hive.json.encode, sparse)
  hive.test_capture("sparse", tostring(err3))
  if ok3 then hive.test_capture("sparse_ok", "true") end
end
`, &JSONModule{})

	assert.False(t, h.capture.Has("string_key_ok"), "encode should reject array-tagged table with string key")
	assert.Contains(t, h.capture.String("string_key"), "array-tagged")
	assert.Contains(t, h.capture.String("string_key"), "non-integer key")

	assert.False(t, h.capture.Has("mixed_ok"))
	assert.Contains(t, h.capture.String("mixed"), "non-integer key")

	assert.False(t, h.capture.Has("sparse_ok"))
	assert.Contains(t, h.capture.String("sparse"), "hole at index 2")
}

func TestJSONEncodeRejectsUnsupportedKeyTypes(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local bool_key = {}
  bool_key[true] = "x"
  local ok1, err1 = pcall(hive.json.encode, bool_key)
  hive.test_capture("bool", tostring(err1))
  if ok1 then hive.test_capture("bool_ok", "true") end

  local table_key = {}
  table_key[{}] = "x"
  local ok2, err2 = pcall(hive.json.encode, table_key)
  hive.test_capture("table", tostring(err2))
  if ok2 then hive.test_capture("table_ok", "true") end
end
`, &JSONModule{})

	assert.False(t, h.capture.Has("bool_ok"))
	assert.Contains(t, h.capture.String("bool"), "boolean as object key")

	assert.False(t, h.capture.Has("table_ok"))
	assert.Contains(t, h.capture.String("table"), "table as object key")
}

func TestJSONEncodeOptsValidation(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
return function(hive)
  local ok1, err1 = pcall(hive.json.encode, {}, { pretty = "true" })
  hive.test_capture("non_bool", tostring(err1))
  if ok1 then hive.test_capture("non_bool_ok", "true") end

  local ok2, err2 = pcall(hive.json.encode, {}, { prety = true })
  hive.test_capture("typo", tostring(err2))
  if ok2 then hive.test_capture("typo_ok", "true") end

  -- Valid opts still works.
  hive.test_capture("valid", hive.json.encode({ foo = 1 }, { pretty = true }))
end
`, &JSONModule{})

	assert.False(t, h.capture.Has("non_bool_ok"))
	assert.Contains(t, h.capture.String("non_bool"), "opts.pretty must be a boolean")

	assert.False(t, h.capture.Has("typo_ok"))
	assert.Contains(t, h.capture.String("typo"), `unknown opts key "prety"`)

	assert.Contains(t, h.capture.String("valid"), "\n")
}

func TestJSONDecodedArrayRoundTripsAfterMutation(t *testing.T) {
	t.Parallel()
	h := newLuaHarness(t, `
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
`, &JSONModule{})

	assert.Equal(t, "[]", h.capture.String("after_drain"))

	var arr []any
	require.NoError(t, json.Unmarshal([]byte(h.capture.String("after_remove_middle")), &arr))
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
