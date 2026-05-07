package lua

import (
	"encoding/json"
	"errors"
	"fmt"

	glua "github.com/yuin/gopher-lua"
)

// JSONModule exposes hive.json.encode, hive.json.decode, and hive.json.array.
//
// The array marker is a per-Register *LTable used as a metatable. Its identity
// (pointer) tags Lua tables that should be encoded as JSON arrays even when
// they are empty or otherwise look like objects to the heuristic walker.
type JSONModule struct {
	// arrayMarker is the metatable installed on tables passed through array().
	// Identity is by pointer; we never read fields off it.
	arrayMarker *glua.LTable
}

// Register attaches encode/decode/array to a fresh hive.json subtable.
func (m *JSONModule) Register(state *glua.LState, hive *glua.LTable) error {
	m.arrayMarker = state.NewTable()

	json := state.NewTable()
	state.SetField(json, "encode", state.NewFunction(m.luaEncode))
	state.SetField(json, "decode", state.NewFunction(m.luaDecode))
	state.SetField(json, "array", state.NewFunction(m.luaArray))
	state.SetField(hive, "json", json)
	return nil
}

// luaArray tags the given table with the array marker and returns it. The
// same table is mutated, so callers can chain or assign as they please.
func (m *JSONModule) luaArray(state *glua.LState) int {
	table := state.CheckTable(1)
	state.SetMetatable(table, m.arrayMarker)
	state.Push(table)
	return 1
}

// luaEncode implements hive.json.encode(value, opts?).
//
// opts is an optional table with a "pretty" boolean. When pretty is true the
// output uses two-space indentation via json.MarshalIndent.
func (m *JSONModule) luaEncode(state *glua.LState) int {
	value := state.CheckAny(1)

	pretty := false
	if state.GetTop() >= 2 {
		opts := state.CheckTable(2)
		if v, ok := opts.RawGetString("pretty").(glua.LBool); ok {
			pretty = bool(v)
		}
	}

	visited := map[*glua.LTable]bool{}
	goValue, err := m.luaToGo(value, visited)
	if err != nil {
		state.RaiseError("hive.json.encode: %s", err.Error())
		return 0
	}

	var (
		bytes []byte
		mErr  error
	)
	if pretty {
		bytes, mErr = json.MarshalIndent(goValue, "", "  ")
	} else {
		bytes, mErr = json.Marshal(goValue)
	}
	if mErr != nil {
		state.RaiseError("hive.json.encode: %s", mErr.Error())
		return 0
	}

	state.Push(glua.LString(bytes))
	return 1
}

// luaDecode implements hive.json.decode(string).
func (m *JSONModule) luaDecode(state *glua.LState) int {
	input := state.CheckString(1)

	var v any
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			state.RaiseError("hive.json.decode: %s at offset %d", syntaxErr.Error(), syntaxErr.Offset)
		case errors.As(err, &typeErr):
			state.RaiseError("hive.json.decode: %s at offset %d", typeErr.Error(), typeErr.Offset)
		default:
			state.RaiseError("hive.json.decode: %s", err.Error())
		}
		return 0
	}

	state.Push(m.goToLua(state, v))
	return 1
}

// luaToGo walks Lua values into Go-native shapes that encoding/json can
// marshal. Tables become []any when the array marker is set or when keys are
// a dense 1..n integer run; otherwise map[string]any.
func (m *JSONModule) luaToGo(lv glua.LValue, visited map[*glua.LTable]bool) (any, error) {
	switch v := lv.(type) {
	case *glua.LNilType:
		return nil, nil
	case glua.LBool:
		return bool(v), nil
	case glua.LNumber:
		return float64(v), nil
	case glua.LString:
		return string(v), nil
	case *glua.LTable:
		if visited[v] {
			return nil, fmt.Errorf("cannot encode cyclic table")
		}
		visited[v] = true
		defer delete(visited, v)
		return m.tableToGo(v, visited)
	case *glua.LFunction:
		return nil, fmt.Errorf("cannot encode lua function")
	case *glua.LUserData:
		return nil, fmt.Errorf("cannot encode lua userdata")
	case *glua.LChannel:
		return nil, fmt.Errorf("cannot encode lua channel")
	default:
		return nil, fmt.Errorf("cannot encode lua value of type %s", lv.Type().String())
	}
}

// tableToGo decides between array and object encoding for a Lua table.
//
// Detection order:
//  1. The array marker metatable is set → []any (preserves empty-array intent).
//  2. All keys are positive integers 1..n with no holes → []any.
//  3. Otherwise → map[string]any.
//
// We index sequentially via RawGetInt because ForEach iteration order is
// undefined; relying on it would race with table layout changes.
func (m *JSONModule) tableToGo(tbl *glua.LTable, visited map[*glua.LTable]bool) (any, error) {
	if tbl.Metatable == m.arrayMarker {
		return m.tableToArray(tbl, visited)
	}

	if isArrayLike(tbl) {
		return m.tableToArray(tbl, visited)
	}

	return m.tableToObject(tbl, visited)
}

// isArrayLike returns true when every key in tbl is a positive integer 1..n
// with no holes and no extra non-array keys. Empty tables are NOT array-like;
// callers must apply the marker rule first to disambiguate them as []any.
func isArrayLike(tbl *glua.LTable) bool {
	n := tbl.Len()
	if n == 0 {
		return false
	}
	for i := 1; i <= n; i++ {
		if tbl.RawGetInt(i) == glua.LNil {
			return false
		}
	}

	// Confirm there are no non-integer or out-of-range keys.
	extra := false
	tbl.ForEach(func(key glua.LValue, _ glua.LValue) {
		if extra {
			return
		}
		num, ok := key.(glua.LNumber)
		if !ok {
			extra = true
			return
		}
		idx := int(num)
		if glua.LNumber(idx) != num || idx < 1 || idx > n {
			extra = true
		}
	})
	return !extra
}

func (m *JSONModule) tableToArray(tbl *glua.LTable, visited map[*glua.LTable]bool) ([]any, error) {
	out := make([]any, 0, tbl.Len())
	for i := 1; i <= tbl.Len(); i++ {
		entry, err := m.luaToGo(tbl.RawGetInt(i), visited)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func (m *JSONModule) tableToObject(tbl *glua.LTable, visited map[*glua.LTable]bool) (map[string]any, error) {
	out := map[string]any{}
	var walkErr error
	tbl.ForEach(func(key glua.LValue, value glua.LValue) {
		if walkErr != nil {
			return
		}
		k := glua.LVAsString(key)
		entry, err := m.luaToGo(value, visited)
		if err != nil {
			walkErr = err
			return
		}
		out[k] = entry
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

// goToLua walks Go values from encoding/json back into Lua. Empty []any
// keeps the array marker so a re-encode round-trips as [] rather than {}.
func (m *JSONModule) goToLua(state *glua.LState, value any) glua.LValue {
	switch v := value.(type) {
	case nil:
		return glua.LNil
	case bool:
		return glua.LBool(v)
	case float64:
		return glua.LNumber(v)
	case string:
		return glua.LString(v)
	case []any:
		tbl := state.NewTable()
		for i, item := range v {
			tbl.RawSetInt(i+1, m.goToLua(state, item))
		}
		// Empty arrays would otherwise round-trip to {}; tag them so a
		// follow-up encode preserves the [] shape.
		if len(v) == 0 {
			state.SetMetatable(tbl, m.arrayMarker)
		}
		return tbl
	case map[string]any:
		tbl := state.NewTable()
		for k, item := range v {
			tbl.RawSetString(k, m.goToLua(state, item))
		}
		return tbl
	default:
		// json.Unmarshal into any only produces the cases above; this is
		// defensive against future encoding/json behaviour changes.
		return glua.LNil
	}
}
