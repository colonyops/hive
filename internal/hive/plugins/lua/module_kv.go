package lua

import (
	"context"
	"database/sql"
	"errors"

	glua "github.com/yuin/gopher-lua"

	"github.com/colonyops/hive/internal/core/kv"
)

// KVModule exposes hive.kv.{get,set,delete} backed by a string-typed
// kv.Scoped store. The scope is applied at construction so the Lua sandbox
// cannot read or stomp keys outside its namespace.
type KVModule struct {
	Store *kv.TypedKV[string]
}

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
		state.RaiseError("kv set %q: %s", key, err)
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
		state.RaiseError("kv get %q: %s", key, err)
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
		state.RaiseError("kv delete %q: %s", key, err)
		return 0
	}
	return 0
}
