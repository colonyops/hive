package plugins

import (
	"maps"
	"sync"

	"github.com/colonyops/hive/internal/core/config"
)

// CommandSet is the canonical merged registry of user commands across system
// defaults, plugin contributions, and user-config overrides. It is concurrent-
// safe: writers from any goroutine push via Set*/Merge*; readers via Lookup
// or All. Merge precedence in reads is user > plugin > system.
//
// Slots are kept separate so writers touch only their own source — a plugin
// re-registering does not need to know about user/system entries to avoid
// clobbering them.
type CommandSet struct {
	mu      sync.RWMutex
	system  map[string]config.UserCommand
	user    map[string]config.UserCommand
	plugins map[string]map[string]config.UserCommand // plugin name -> commands
}

// NewCommandSet constructs a CommandSet seeded with system and user commands.
// Either map may be nil (treated as empty). System and user are immutable
// after construction; if a config-reload story arrives later, add setters then.
func NewCommandSet(system, user map[string]config.UserCommand) *CommandSet {
	return &CommandSet{
		system:  cloneCommands(system),
		user:    cloneCommands(user),
		plugins: map[string]map[string]config.UserCommand{},
	}
}

// SetPlugin replaces the named plugin's slot. Used by static plugins at
// InitAll time and to clear a per-plugin slot during a Plugin re-init.
// Pass nil cmds to clear (removes the entry from plugins map).
func (s *CommandSet) SetPlugin(name string, cmds map[string]config.UserCommand) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cmds == nil {
		delete(s.plugins, name)
		return
	}
	s.plugins[name] = cloneCommands(cmds)
}

// MergePlugin merges cmds into the named plugin's slot. Existing entries
// not in cmds are preserved; entries in cmds replace any existing same-named
// entry. Used by the Lua plugin on each hive.commands(...) call.
func (s *CommandSet) MergePlugin(name string, cmds map[string]config.UserCommand) {
	if cmds == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	slot, ok := s.plugins[name]
	if !ok {
		slot = make(map[string]config.UserCommand, len(cmds))
		s.plugins[name] = slot
	}
	maps.Copy(slot, cmds)
}

// Plugin returns a defensive copy of the named plugin's slot, or nil if the
// plugin has no slot.
func (s *CommandSet) Plugin(name string) map[string]config.UserCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, ok := s.plugins[name]
	if !ok {
		return nil
	}
	return cloneCommands(slot)
}

// Lookup returns a single merged command by name, applying precedence
// (user > plugin > system). The boolean is false if no source defines name.
// Hot path: called per keystroke from the keybinding resolver. One RLock,
// up to three map lookups, no allocation.
func (s *CommandSet) Lookup(name string) (config.UserCommand, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if cmd, ok := s.user[name]; ok {
		return cmd, true
	}
	for _, slot := range s.plugins {
		if cmd, ok := slot[name]; ok {
			return cmd, true
		}
	}
	if cmd, ok := s.system[name]; ok {
		return cmd, true
	}
	return config.UserCommand{}, false
}

// All returns a defensive copy of the fully merged command map.
func (s *CommandSet) All() map[string]config.UserCommand {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]config.UserCommand, len(s.system)+len(s.user))
	maps.Copy(result, s.system)
	for _, slot := range s.plugins {
		maps.Copy(result, slot)
	}
	maps.Copy(result, s.user)
	return result
}

// cloneCommands returns a shallow copy of m, or an empty map if m is nil.
// UserCommand contains slice fields (Windows, Form, Scope); we deliberately
// do NOT deep-copy those — callers are expected to treat command values as
// immutable once published into the set.
func cloneCommands(m map[string]config.UserCommand) map[string]config.UserCommand {
	out := make(map[string]config.UserCommand, len(m))
	maps.Copy(out, m)
	return out
}
