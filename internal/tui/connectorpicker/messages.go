package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/connectors"
)

// connectorSearchResultMsg carries a completed Search response back to the
// picker. Gen identifies the picker instance that issued the search and
// Query is compared against the picker's current input value on receipt,
// so stale (superseded, or from a previously closed picker) results are
// discarded.
type connectorSearchResultMsg struct {
	Gen   int64
	Query string
	Items []connectors.Item
}

// connectorSearchErrorMsg carries a Search failure back to the picker.
type connectorSearchErrorMsg struct {
	Gen   int64
	Query string
	Err   error
}

// connectorDetailResultMsg carries a completed FetchDetail response back to
// the picker, keyed by item ID. Gen guards against a late result from a
// previously closed picker (possibly for a different scope) poisoning the
// current picker's detail cache.
type connectorDetailResultMsg struct {
	Gen    int64
	ID     string
	Detail connectors.Detail
}

// connectorDetailErrorMsg carries a FetchDetail failure back to the picker.
type connectorDetailErrorMsg struct {
	Gen int64
	ID  string
	Err error
}

// connectorSearchDebounceMsg fires after the manifest-configured debounce
// delay elapses for a remote-mode query change. If Gen matches the current
// picker and Query still matches its input value when received, a real
// Search call is issued.
type connectorSearchDebounceMsg struct {
	Gen   int64
	Query string
}

// defaultConnectorSearchDebounce is used when a remote-search manifest does
// not configure a debounce delay.
const defaultConnectorSearchDebounce = 300 * time.Millisecond

// connectorSearchCmd issues a Search call against conn and wraps the result
// into a connectorSearchResultMsg/connectorSearchErrorMsg.
func connectorSearchCmd(gen int64, conn connectors.Connector, scope, query string) tea.Cmd {
	return func() tea.Msg {
		result, err := conn.Search(context.Background(), connectors.SearchParams{
			Query: query,
			Scope: scope,
		})
		if err != nil {
			return connectorSearchErrorMsg{Gen: gen, Query: query, Err: err}
		}
		return connectorSearchResultMsg{Gen: gen, Query: query, Items: result.Items}
	}
}

// connectorFetchDetailCmd issues a FetchDetail call for item and wraps the
// result into a connectorDetailResultMsg/connectorDetailErrorMsg.
func connectorFetchDetailCmd(gen int64, conn connectors.Connector, scope string, item connectors.Item) tea.Cmd {
	return func() tea.Msg {
		detail, err := conn.FetchDetail(context.Background(), connectors.FetchDetailParams{
			ID:    item.ID,
			Scope: scope,
			URI:   item.URI,
		})
		if err != nil {
			return connectorDetailErrorMsg{Gen: gen, ID: item.ID, Err: err}
		}
		return connectorDetailResultMsg{Gen: gen, ID: item.ID, Detail: detail}
	}
}

// connectorDebounceCmd schedules a connectorSearchDebounceMsg for query
// after delay.
func connectorDebounceCmd(gen int64, query string, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return connectorSearchDebounceMsg{Gen: gen, Query: query}
	})
}
