package sourcepicker

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/sources"
)

// --- Tab lifecycle messages ---

// sourceTabReadyMsg carries a fully initialized tab (manifest + initial
// items) back to the picker after the Available+Initialize+Search pipeline
// completes.
type sourceTabReadyMsg struct {
	Gen      int64
	SourceID string
	Manifest sources.Manifest
	Items    []sources.Item
}

// sourceTabErrorMsg carries a tab initialization failure.
type sourceTabErrorMsg struct {
	Gen      int64
	SourceID string
	Err      error
}

// --- Search messages (for remote-mode search within a loaded tab) ---

type sourceSearchResultMsg struct {
	Gen      int64
	SourceID string
	Query    string
	Items    []sources.Item
}

type sourceSearchErrorMsg struct {
	Gen      int64
	SourceID string
	Query    string
	Err      error
}

// sourceSearchDebounceMsg fires after the remote-search debounce delay;
// the picker only dispatches a Search when Gen, SourceID, and Query still
// match the tab's current state (i.e. the user stopped typing).
type sourceSearchDebounceMsg struct {
	Gen      int64
	SourceID string
	Query    string
}

func sourceSearchCmd(gen int64, conn sources.Source, sourceID, scope, dir, query string) tea.Cmd {
	return func() tea.Msg {
		result, err := conn.Search(context.Background(), sources.SearchParams{
			Query: query,
			Scope: scope,
			Dir:   dir,
		})
		if err != nil {
			return sourceSearchErrorMsg{Gen: gen, SourceID: sourceID, Query: query, Err: err}
		}
		return sourceSearchResultMsg{Gen: gen, SourceID: sourceID, Query: query, Items: result.Items}
	}
}

// Msg marks the picker's async messages so the parent model can route them
// with a single type-switch case.
type Msg interface{ isPickerMsg() }

func (sourceTabReadyMsg) isPickerMsg()       {}
func (sourceTabErrorMsg) isPickerMsg()       {}
func (sourceSearchResultMsg) isPickerMsg()   {}
func (sourceSearchErrorMsg) isPickerMsg()    {}
func (sourceSearchDebounceMsg) isPickerMsg() {}

// SpinnerTickMsg re-exports spinner.TickMsg so the parent can route it.
type SpinnerTickMsg = spinner.TickMsg
