package sourcepicker

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/sources"
)

// sourceSearchResultMsg carries a completed Search response back to the
// picker. Gen identifies the picker instance that issued the search and
// Query is compared against the picker's current input value on receipt,
// so stale (superseded, or from a previously closed picker) results are
// discarded.
type sourceSearchResultMsg struct {
	Gen   int64
	Query string
	Items []sources.Item
}

// sourceSearchErrorMsg carries a Search failure back to the picker.
type sourceSearchErrorMsg struct {
	Gen   int64
	Query string
	Err   error
}

// sourceDetailResultMsg carries a completed FetchDetail response back to
// the picker, keyed by item ID. Gen guards against a late result from a
// previously closed picker (possibly for a different scope) poisoning the
// current picker's detail cache.
type sourceDetailResultMsg struct {
	Gen    int64
	ID     string
	Detail sources.Detail
}

// sourceDetailErrorMsg carries a FetchDetail failure back to the picker.
type sourceDetailErrorMsg struct {
	Gen int64
	ID  string
	Err error
}

// sourceSearchDebounceMsg fires after the manifest-configured debounce
// delay elapses for a remote-mode query change. If Gen matches the current
// picker and Query still matches its input value when received, a real
// Search call is issued.
type sourceSearchDebounceMsg struct {
	Gen   int64
	Query string
}

// defaultSourceSearchDebounce is used when a remote-search manifest does
// not configure a debounce delay.
const defaultSourceSearchDebounce = 300 * time.Millisecond

// sourceSearchCmd issues a Search call against conn and wraps the result
// into a sourceSearchResultMsg/sourceSearchErrorMsg.
func sourceSearchCmd(gen int64, conn sources.Source, scope, query string) tea.Cmd {
	return func() tea.Msg {
		result, err := conn.Search(context.Background(), sources.SearchParams{
			Query: query,
			Scope: scope,
		})
		if err != nil {
			return sourceSearchErrorMsg{Gen: gen, Query: query, Err: err}
		}
		return sourceSearchResultMsg{Gen: gen, Query: query, Items: result.Items}
	}
}

// sourceFetchDetailCmd issues a FetchDetail call for item and wraps the
// result into a sourceDetailResultMsg/sourceDetailErrorMsg.
func sourceFetchDetailCmd(gen int64, conn sources.Source, scope string, item sources.Item) tea.Cmd {
	return func() tea.Msg {
		detail, err := conn.FetchDetail(context.Background(), sources.FetchDetailParams{
			ID:    item.ID,
			Scope: scope,
			URI:   item.URI,
		})
		if err != nil {
			return sourceDetailErrorMsg{Gen: gen, ID: item.ID, Err: err}
		}
		return sourceDetailResultMsg{Gen: gen, ID: item.ID, Detail: detail}
	}
}

// sourceDebounceCmd schedules a sourceSearchDebounceMsg for query
// after delay.
func sourceDebounceCmd(gen int64, query string, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return sourceSearchDebounceMsg{Gen: gen, Query: query}
	})
}

// Msg marks the picker's async result messages (search/detail results,
// errors, debounce ticks) so the parent model can route top-level tea.Msg
// values to the active picker with a single type-switch case, without
// importing the individual message types.
type Msg interface{ isPickerMsg() }

func (sourceSearchResultMsg) isPickerMsg()   {}
func (sourceSearchErrorMsg) isPickerMsg()    {}
func (sourceSearchDebounceMsg) isPickerMsg() {}
func (sourceDetailResultMsg) isPickerMsg()   {}
func (sourceDetailErrorMsg) isPickerMsg()    {}
