package lua

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rs/zerolog"
	glua "github.com/yuin/gopher-lua"

	"github.com/colonyops/hive/internal/core/kv"
)

// KVModule exposes hive.kv.{get,set,delete} backed by a string-typed
// kv.Scoped store. The scope is applied at construction so the Lua sandbox
// cannot read or stomp keys outside its namespace.
//
// Store errors are both raised to Lua and logged at Warn so operators can
// see them even when the plugin pcall's the call to suppress the error.
type KVModule struct {
	Store  *kv.TypedKV[string]
	Logger zerolog.Logger
}

// Register attaches get/set/delete to a fresh hive.kv subtable.
func (m *KVModule) Register(state *glua.LState, hive *glua.LTable) error {
	table := state.NewTable()
	state.SetField(table, "get", state.NewFunction(m.get))
	state.SetField(table, "set", state.NewFunction(m.set))
	state.SetField(table, "delete", state.NewFunction(m.delete))
	state.SetField(hive, "kv", table)
	return nil
}

func (m *KVModule) set(state *glua.LState) int {
	key := state.CheckString(1)
	value := state.CheckString(2)
	if key == "" {
		state.ArgError(1, "key cannot be empty")
		return 0
	}
	if err := m.Store.Set(context.Background(), key, value); err != nil {
		m.Logger.Warn().Str("op", "set").Str("key", key).Err(err).Msg("hive.kv: store error")
		state.RaiseError("hive.kv.set %q: %s", key, err)
		return 0
	}
	return 0
}

func (m *KVModule) get(state *glua.LState) int {
	key := state.CheckString(1)
	if key == "" {
		state.ArgError(1, "key cannot be empty")
		return 0
	}
	value, err := m.Store.Get(context.Background(), key)
	if errors.Is(err, sql.ErrNoRows) {
		state.Push(glua.LNil)
		return 1
	}
	if err != nil {
		m.Logger.Warn().Str("op", "get").Str("key", key).Err(err).Msg("hive.kv: store error")
		state.RaiseError("hive.kv.get %q: %s", key, err)
		return 0
	}
	state.Push(glua.LString(value))
	return 1
}

func (m *KVModule) delete(state *glua.LState) int {
	key := state.CheckString(1)
	if key == "" {
		state.ArgError(1, "key cannot be empty")
		return 0
	}
	if err := m.Store.Delete(context.Background(), key); err != nil {
		m.Logger.Warn().Str("op", "delete").Str("key", key).Err(err).Msg("hive.kv: store error")
		state.RaiseError("hive.kv.delete %q: %s", key, err)
		return 0
	}
	return 0
}
