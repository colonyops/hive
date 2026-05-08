package lua

import (
	"fmt"

	glua "github.com/yuin/gopher-lua"
)

// luaToGo walks a Lua value into encoding/json-compatible Go shapes.
//
// Tables become []any when array-tagged (arrayMarker matches the table's
// metatable identity) or when keys are a dense 1..n integer run; otherwise
// map[string]any. Cycles produce an error rather than recurse forever.
// visited tracks table identities across recursion; pass an empty map at
// the top level.
//
// arrayMarker may be nil — without it, only the integer-key heuristic
// applies, and empty tables encode as map[string]any.
func luaToGo(v glua.LValue, arrayMarker *glua.LTable, visited map[*glua.LTable]bool) (any, error) {
	switch v := v.(type) {
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
		return tableToGo(v, arrayMarker, visited)
	case *glua.LFunction:
		return nil, fmt.Errorf("cannot encode lua function")
	case *glua.LUserData:
		return nil, fmt.Errorf("cannot encode lua userdata")
	case *glua.LChannel:
		return nil, fmt.Errorf("cannot encode lua channel")
	default:
		return nil, fmt.Errorf("cannot encode lua value of type %s", v.Type().String())
	}
}

// tableToGo decides between array and object encoding for a Lua table.
//
// Detection order:
//  1. arrayMarker is non-nil and matches the table's metatable → []any
//     (preserves empty-array intent and rejects shape contradictions).
//  2. All keys are positive integers 1..n with no holes → []any.
//  3. Otherwise → map[string]any.
func tableToGo(tbl *glua.LTable, arrayMarker *glua.LTable, visited map[*glua.LTable]bool) (any, error) {
	if arrayMarker != nil && tbl.Metatable == arrayMarker {
		if err := arrayShapeError(tbl); err != nil {
			return nil, fmt.Errorf("array-tagged table %s", err.Error())
		}
		return tableToArray(tbl, arrayMarker, visited)
	}

	if isArrayLike(tbl) {
		return tableToArray(tbl, arrayMarker, visited)
	}

	return tableToObject(tbl, arrayMarker, visited)
}

// arrayShapeError returns nil when tbl's keys are exactly the positive
// integers 1..n with no holes (empty tables satisfy the contract).
// Otherwise it returns an error describing the first violation. Used by
// the marker path to surface inconsistent claims (e.g.
// hive.json.array({foo="x"})) as errors instead of silently dropping data.
func arrayShapeError(tbl *glua.LTable) error {
	n := tbl.Len()
	for i := 1; i <= n; i++ {
		if tbl.RawGetInt(i) == glua.LNil {
			return fmt.Errorf("has hole at index %d", i)
		}
	}
	var keyErr error
	tbl.ForEach(func(key glua.LValue, _ glua.LValue) {
		if keyErr != nil {
			return
		}
		num, ok := key.(glua.LNumber)
		if !ok {
			keyErr = fmt.Errorf("has non-integer key %q", key.String())
			return
		}
		idx := int(num)
		if glua.LNumber(idx) != num || idx < 1 || idx > n {
			keyErr = fmt.Errorf("has out-of-range key %v", num)
		}
	})
	return keyErr
}

// isArrayLike returns true when tbl matches the heuristic for an untagged
// array. Empty tables fail the heuristic so the marker rule can
// disambiguate them as []any vs. {}.
func isArrayLike(tbl *glua.LTable) bool {
	if tbl.Len() == 0 {
		return false
	}
	return arrayShapeError(tbl) == nil
}

func tableToArray(tbl *glua.LTable, arrayMarker *glua.LTable, visited map[*glua.LTable]bool) ([]any, error) {
	out := make([]any, 0, tbl.Len())
	for i := 1; i <= tbl.Len(); i++ {
		entry, err := luaToGo(tbl.RawGetInt(i), arrayMarker, visited)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

func tableToObject(tbl *glua.LTable, arrayMarker *glua.LTable, visited map[*glua.LTable]bool) (map[string]any, error) {
	out := map[string]any{}
	var walkErr error
	tbl.ForEach(func(key glua.LValue, value glua.LValue) {
		if walkErr != nil {
			return
		}
		k, err := objectKey(key)
		if err != nil {
			walkErr = err
			return
		}
		entry, err := luaToGo(value, arrayMarker, visited)
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

// objectKey coerces a Lua table key to a JSON object key. Only string and
// number keys are supported; other types (booleans, tables, functions,
// userdata) would silently collide on the empty string via LVAsString and
// drop entries, so they're rejected with an explicit error.
func objectKey(key glua.LValue) (string, error) {
	switch k := key.(type) {
	case glua.LString:
		return string(k), nil
	case glua.LNumber:
		return glua.LVAsString(key), nil
	default:
		return "", fmt.Errorf("cannot use %s as object key", key.Type().String())
	}
}

// goToLua walks an encoding/json-compatible Go value back into Lua.
//
// arrayMarker, when non-nil, is set as the metatable on every []any-derived
// table so a re-encode preserves the [] shape regardless of mutations
// (table.remove emptying it, etc.) that would otherwise flunk the
// integer-key heuristic. Pass nil if marker propagation isn't needed.
func goToLua(state *glua.LState, v any, arrayMarker *glua.LTable) glua.LValue {
	switch v := v.(type) {
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
			tbl.RawSetInt(i+1, goToLua(state, item, arrayMarker))
		}
		if arrayMarker != nil {
			state.SetMetatable(tbl, arrayMarker)
		}
		return tbl
	case map[string]any:
		tbl := state.NewTable()
		for k, item := range v {
			tbl.RawSetString(k, goToLua(state, item, arrayMarker))
		}
		return tbl
	default:
		// json.Unmarshal into any only produces the cases above; this is
		// defensive against future encoding/json behaviour changes.
		return glua.LNil
	}
}
