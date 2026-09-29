package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/core/kv"
)

// kvKeysLoadedMsg is sent when KV keys are fetched.
type kvKeysLoadedMsg struct {
	keys []string
	err  error
}

// kvEntryLoadedMsg is sent when a KV entry is fetched for preview.
type kvEntryLoadedMsg struct {
	entry kv.Entry
	err   error
}

// loadKVKeys returns a tea.Cmd that fetches all KV keys.
func (m Model) loadKVKeys() tea.Cmd {
	if m.kvStore == nil {
		return nil
	}
	store := m.kvStore
	return func() tea.Msg {
		keys, err := store.ListKeys(context.Background())
		return kvKeysLoadedMsg{keys: keys, err: err}
	}
}

// loadKVEntry returns a tea.Cmd that fetches a raw KV entry.
func (m Model) loadKVEntry(key string) tea.Cmd {
	if m.kvStore == nil || key == "" {
		return nil
	}
	store := m.kvStore
	return func() tea.Msg {
		entry, err := store.GetRaw(context.Background(), key)
		return kvEntryLoadedMsg{entry: entry, err: err}
	}
}
