package lua

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	glua "github.com/yuin/gopher-lua"
)

// newConvertState returns a closed-over LState for direct conversion-helper
// tests. The state never runs Lua — it only owns the table allocator.
func newConvertState(t *testing.T) *glua.LState {
	t.Helper()
	state := glua.NewState()
	t.Cleanup(state.Close)
	return state
}

// asLuaNumber unwraps an LNumber to float64 for InDelta comparisons.
func asLuaNumber(t *testing.T, v glua.LValue) float64 {
	t.Helper()
	n, ok := v.(glua.LNumber)
	require.True(t, ok, "expected LNumber, got %T", v)
	return float64(n)
}

func TestLuaToGo_Primitives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   glua.LValue
		want any
	}{
		{"nil", glua.LNil, nil},
		{"true", glua.LBool(true), true},
		{"false", glua.LBool(false), false},
		{"int number", glua.LNumber(42), float64(42)},
		{"float number", glua.LNumber(3.5), float64(3.5)},
		{"empty string", glua.LString(""), ""},
		{"string", glua.LString("hello"), "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := luaToGo(tt.in, nil, map[*glua.LTable]bool{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLuaToGo_TableAsArrayByHeuristic(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	tbl := state.NewTable()
	tbl.RawSetInt(1, glua.LString("a"))
	tbl.RawSetInt(2, glua.LString("b"))
	tbl.RawSetInt(3, glua.LString("c"))

	got, err := luaToGo(tbl, nil, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, []any{"a", "b", "c"}, got)
}

func TestLuaToGo_EmptyTableIsObjectWithoutMarker(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	got, err := luaToGo(state.NewTable(), nil, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, got, "an empty untagged table is an object, not an array")
}

func TestLuaToGo_EmptyTableIsArrayWithMatchingMarker(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	marker := state.NewTable()
	tbl := state.NewTable()
	state.SetMetatable(tbl, marker)

	got, err := luaToGo(tbl, marker, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, []any{}, got)
}

func TestLuaToGo_TableAsObject(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	tbl := state.NewTable()
	tbl.RawSetString("foo", glua.LNumber(1))
	tbl.RawSetString("bar", glua.LString("baz"))

	got, err := luaToGo(tbl, nil, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"foo": float64(1), "bar": "baz"}, got)
}

func TestLuaToGo_NestedMixed(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	inner := state.NewTable()
	inner.RawSetInt(1, glua.LNumber(10))
	inner.RawSetInt(2, glua.LNumber(20))

	outer := state.NewTable()
	outer.RawSetString("nums", inner)
	outer.RawSetString("flag", glua.LBool(true))

	got, err := luaToGo(outer, nil, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"nums": []any{float64(10), float64(20)},
		"flag": true,
	}, got)
}

func TestLuaToGo_RejectsCycle(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	tbl := state.NewTable()
	tbl.RawSetString("self", tbl)

	_, err := luaToGo(tbl, nil, map[*glua.LTable]bool{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic")
}

func TestLuaToGo_RejectsUnsupportedTypes(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	tests := []struct {
		name string
		in   glua.LValue
		want string
	}{
		{"function", state.NewFunction(func(*glua.LState) int { return 0 }), "function"},
		{"userdata", state.NewUserData(), "userdata"},
		{"channel", glua.LChannel(make(chan glua.LValue)), "channel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := luaToGo(tt.in, nil, map[*glua.LTable]bool{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestLuaToGo_ArrayMarkerRejectsObjectShape(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	marker := state.NewTable()
	tbl := state.NewTable()
	tbl.RawSetString("foo", glua.LString("x"))
	state.SetMetatable(tbl, marker)

	_, err := luaToGo(tbl, marker, map[*glua.LTable]bool{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "array-tagged")
	assert.Contains(t, err.Error(), "non-integer key")
}

func TestLuaToGo_ArrayMarkerRejectsHole(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	marker := state.NewTable()
	tbl := state.NewTable()
	tbl.RawSetInt(1, glua.LString("a"))
	tbl.RawSetInt(3, glua.LString("c")) // hole at 2
	state.SetMetatable(tbl, marker)

	_, err := luaToGo(tbl, marker, map[*glua.LTable]bool{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hole at index 2")
}

func TestLuaToGo_NonMatchingMarkerIgnored(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	// Table is tagged with a different marker than the one we pass in,
	// so the marker rule does not apply and the heuristic takes over.
	otherMarker := state.NewTable()
	tbl := state.NewTable()
	state.SetMetatable(tbl, otherMarker)

	ourMarker := state.NewTable()
	got, err := luaToGo(tbl, ourMarker, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, got, "empty table without our marker should be an object")
}

func TestLuaToGo_RejectsUnsupportedKeyTypes(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	t.Run("bool key", func(t *testing.T) {
		tbl := state.NewTable()
		tbl.RawSet(glua.LBool(true), glua.LString("x"))
		_, err := luaToGo(tbl, nil, map[*glua.LTable]bool{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boolean as object key")
	})

	t.Run("table key", func(t *testing.T) {
		tbl := state.NewTable()
		tbl.RawSet(state.NewTable(), glua.LString("x"))
		_, err := luaToGo(tbl, nil, map[*glua.LTable]bool{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "table as object key")
	})
}

func TestGoToLua_Primitives(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	tests := []struct {
		name string
		in   any
		want glua.LValue
	}{
		{"nil", nil, glua.LNil},
		{"true", true, glua.LBool(true)},
		{"false", false, glua.LBool(false)},
		{"float", float64(3.5), glua.LNumber(3.5)},
		{"int as float", float64(42), glua.LNumber(42)},
		{"string", "hello", glua.LString("hello")},
		{"empty string", "", glua.LString("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goToLua(state, tt.in, nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGoToLua_Array(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	got := goToLua(state, []any{"a", "b", "c"}, nil)

	tbl, ok := got.(*glua.LTable)
	require.True(t, ok)
	assert.Equal(t, glua.LString("a"), tbl.RawGetInt(1))
	assert.Equal(t, glua.LString("b"), tbl.RawGetInt(2))
	assert.Equal(t, glua.LString("c"), tbl.RawGetInt(3))
}

func TestGoToLua_ArrayWithMarker(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	marker := state.NewTable()
	got := goToLua(state, []any{}, marker)

	tbl, ok := got.(*glua.LTable)
	require.True(t, ok)
	assert.Same(t, marker, tbl.Metatable, "empty []any should be tagged with the marker")
}

func TestGoToLua_ArrayWithoutMarkerIsUntagged(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	got := goToLua(state, []any{"x"}, nil)

	tbl, ok := got.(*glua.LTable)
	require.True(t, ok)
	// Fresh tables carry LNil as their metatable (not Go nil); a "no
	// metatable" assertion checks for that sentinel value.
	assert.Equal(t, glua.LNil, tbl.Metatable, "no marker means no metatable")
}

func TestGoToLua_Object(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	got := goToLua(state, map[string]any{"foo": float64(1), "bar": "baz"}, nil)

	tbl, ok := got.(*glua.LTable)
	require.True(t, ok)
	assert.InDelta(t, 1.0, asLuaNumber(t, tbl.RawGetString("foo")), 0)
	assert.Equal(t, glua.LString("baz"), tbl.RawGetString("bar"))
}

func TestGoToLua_NestedMixed(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	got := goToLua(state, map[string]any{
		"nums": []any{float64(10), float64(20)},
		"meta": map[string]any{"x": true},
	}, nil)

	tbl, ok := got.(*glua.LTable)
	require.True(t, ok)

	nums, ok := tbl.RawGetString("nums").(*glua.LTable)
	require.True(t, ok)
	assert.InDelta(t, 10.0, asLuaNumber(t, nums.RawGetInt(1)), 0)
	assert.InDelta(t, 20.0, asLuaNumber(t, nums.RawGetInt(2)), 0)

	meta, ok := tbl.RawGetString("meta").(*glua.LTable)
	require.True(t, ok)
	assert.Equal(t, glua.LBool(true), meta.RawGetString("x"))
}

func TestGoToLua_UnsupportedType(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)

	// goToLua only handles types encoding/json yields when unmarshaling
	// into any. Anything else falls through to LNil — defensive against
	// future encoding/json behaviour changes.
	got := goToLua(state, struct{ X int }{X: 1}, nil)
	assert.Equal(t, glua.LNil, got)
}

func TestRoundTrip_PreservesArrayShape(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)
	marker := state.NewTable()

	// Encode a Go []any to Lua, then decode back.
	original := []any{"a", "b", "c"}
	luaVal := goToLua(state, original, marker)
	got, err := luaToGo(luaVal, marker, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestRoundTrip_EmptyArrayWithMarker(t *testing.T) {
	t.Parallel()
	state := newConvertState(t)
	marker := state.NewTable()

	// Without the marker, an empty []any would round-trip as
	// map[string]any{}; the marker preserves array intent.
	luaVal := goToLua(state, []any{}, marker)
	got, err := luaToGo(luaVal, marker, map[*glua.LTable]bool{})
	require.NoError(t, err)
	assert.Equal(t, []any{}, got)
}
