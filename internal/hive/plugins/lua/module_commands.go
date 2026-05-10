package lua

import (
	"fmt"
	"strconv"

	"github.com/colonyops/hive/internal/core/config"
	"github.com/colonyops/hive/internal/hive/plugins"
	glua "github.com/yuin/gopher-lua"
)

// CommandsModule exposes `hive.commands(table)` to Lua plugins. Each call
// merges its entries into the plugin's slot in the shared CommandSet via
// MergePlugin. Registrations from any Lua context (entrypoint, ticker
// callback, deferred work) are supported; re-registering an existing name
// replaces it (last-write-wins).
//
// Concurrency: the dispatcher goroutine is the sole writer through this
// module. CommandSet.MergePlugin takes its own write lock and is safe to
// call from any goroutine.
type CommandsModule struct {
	PluginName string
	Set        *plugins.CommandSet
}

// Register exposes hive.commands(map) and lazily allocates the
// accumulator if a previous Register hasn't already done so.
func (m *CommandsModule) Register(state *glua.LState, hive *glua.LTable) error {
	state.SetField(hive, "commands", state.NewFunction(func(state *glua.LState) int {
		table := state.CheckTable(1)
		next, err := commandsFromTable(table)
		if err != nil {
			state.RaiseError("hive.commands: %s", err.Error())
			return 0
		}
		m.Set.MergePlugin(m.PluginName, next)
		return 0
	}))
	return nil
}

func commandsFromTable(commandsTable *glua.LTable) (map[string]config.UserCommand, error) {
	commands := make(map[string]config.UserCommand)
	var parseErr error

	commandsTable.ForEach(func(key, value glua.LValue) {
		if parseErr != nil {
			return
		}

		name, ok := key.(glua.LString)
		if !ok {
			parseErr = fmt.Errorf("command names must be strings")
			return
		}

		commandTable, ok := value.(*glua.LTable)
		if !ok {
			parseErr = fmt.Errorf("command %q must be a table", key.String())
			return
		}

		cmd, err := userCommandFromLua(string(name), commandTable)
		if err != nil {
			parseErr = err
			return
		}

		commands[string(name)] = cmd
	})

	return commands, parseErr
}

// Adding a new field requires updating both UserCommand in config and the
// switch below; the explicit cases for action/windows/options/form keep them
// rejected at the v1 boundary.
func userCommandFromLua(name string, commandTable *glua.LTable) (config.UserCommand, error) {
	var cmd config.UserCommand
	var parseErr error

	commandTable.ForEach(func(key, _ glua.LValue) {
		if parseErr != nil {
			return
		}

		fieldName, ok := key.(glua.LString)
		if !ok {
			parseErr = fmt.Errorf("command %q field names must be strings", name)
			return
		}

		field := string(fieldName)
		switch field {
		case "sh":
			sh, err := luaStringField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Sh = sh
		case "help":
			help, err := luaStringField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Help = help
		case "confirm":
			confirm, err := luaStringField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Confirm = confirm
		case "silent":
			silent, err := luaBoolField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Silent = silent
		case "scope":
			scope, err := luaStringSliceField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Scope = scope
		case "exit":
			exit, err := luaExitField(commandTable, field)
			if err != nil {
				parseErr = fmt.Errorf("command %q: %w", name, err)
				return
			}
			cmd.Exit = exit
		case "action", "windows", "options", "form":
			parseErr = fmt.Errorf("command %q: field %q is not supported by lua plugins (supported: sh, help, scope, confirm, silent, exit)", name, field)
		default:
			parseErr = fmt.Errorf("command %q: unsupported field %q (supported: sh, help, scope, confirm, silent, exit)", name, field)
		}
	})

	if parseErr != nil {
		return config.UserCommand{}, parseErr
	}

	field := fmt.Sprintf("command %q", name)
	if err := config.ValidateUserCommandBasic(field, name, cmd).ToError(); err != nil {
		return config.UserCommand{}, err
	}
	if err := config.ValidateUserCommandTemplates(field, cmd).ToError(); err != nil {
		return config.UserCommand{}, err
	}

	return cmd, nil
}

// luaCommandField reads field off table after applying the policy shared
// by every command-table reader: an absent field returns LNil, and a
// callback value is rejected because the v1 command schema is data-only.
// Typed readers below dispatch on the returned LValue.
func luaCommandField(table *glua.LTable, field string) (glua.LValue, error) {
	value := table.RawGetString(field)
	if value.Type() == glua.LTFunction {
		return nil, fmt.Errorf("field %q does not support callback values", field)
	}
	return value, nil
}

func luaStringField(table *glua.LTable, field string) (string, error) {
	value, err := luaCommandField(table, field)
	if err != nil {
		return "", err
	}
	if value == glua.LNil {
		return "", nil
	}
	str, ok := value.(glua.LString)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", field)
	}
	return string(str), nil
}

func luaBoolField(table *glua.LTable, field string) (bool, error) {
	value, err := luaCommandField(table, field)
	if err != nil {
		return false, err
	}
	if value == glua.LNil {
		return false, nil
	}
	boolean, ok := value.(glua.LBool)
	if !ok {
		return false, fmt.Errorf("field %q must be a boolean", field)
	}
	return bool(boolean), nil
}

func luaStringSliceField(table *glua.LTable, field string) ([]string, error) {
	value, err := luaCommandField(table, field)
	if err != nil {
		return nil, err
	}
	if value == glua.LNil {
		return nil, nil
	}
	list, ok := value.(*glua.LTable)
	if !ok {
		return nil, fmt.Errorf("field %q must be a table", field)
	}

	var values []string
	var parseErr error
	list.ForEach(func(_ glua.LValue, value glua.LValue) {
		if parseErr != nil {
			return
		}
		str, ok := value.(glua.LString)
		if !ok {
			parseErr = fmt.Errorf("field %q entries must be strings", field)
			return
		}
		values = append(values, string(str))
	})
	return values, parseErr
}

func luaExitField(table *glua.LTable, field string) (string, error) {
	value, err := luaCommandField(table, field)
	if err != nil {
		return "", err
	}
	switch v := value.(type) {
	case *glua.LNilType:
		return "", nil
	case glua.LString:
		return string(v), nil
	case glua.LBool:
		return strconv.FormatBool(bool(v)), nil
	default:
		return "", fmt.Errorf("field %q must be a string or boolean", field)
	}
}
