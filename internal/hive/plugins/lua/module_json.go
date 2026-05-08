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
		p, err := parseEncodeOpts(opts)
		if err != nil {
			state.RaiseError("hive.json.encode: %s", err.Error())
			return 0
		}
		pretty = p
	}

	goValue, err := luaToGo(value, m.arrayMarker, map[*glua.LTable]bool{})
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

	state.Push(goToLua(state, v, m.arrayMarker))
	return 1
}

// parseEncodeOpts validates the optional opts table for hive.json.encode.
// Unknown keys and non-bool values for known keys are rejected so typos
// surface immediately rather than silently producing the wrong output.
func parseEncodeOpts(opts *glua.LTable) (pretty bool, err error) {
	var walkErr error
	opts.ForEach(func(key glua.LValue, value glua.LValue) {
		if walkErr != nil {
			return
		}
		name, ok := key.(glua.LString)
		if !ok {
			walkErr = fmt.Errorf("opts key must be a string")
			return
		}
		switch string(name) {
		case "pretty":
			b, ok := value.(glua.LBool)
			if !ok {
				walkErr = fmt.Errorf("opts.pretty must be a boolean")
				return
			}
			pretty = bool(b)
		default:
			walkErr = fmt.Errorf("unknown opts key %q", string(name))
		}
	})
	return pretty, walkErr
}
