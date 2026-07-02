package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/colonyops/hive/internal/connectors"
)

// connectorSearchResultMsg carries a completed Search response back to the
// picker. Query is compared against the picker's current input value on
// receipt to discard stale (superseded) results.
type connectorSearchResultMsg struct {
	Query string
	Items []connectors.Item
}

// connectorSearchErrorMsg carries a Search failure back to the picker.
type connectorSearchErrorMsg struct {
	Query string
	Err   error
}

// connectorDetailResultMsg carries a completed FetchDetail response back to
// the picker, keyed by item ID.
type connectorDetailResultMsg struct {
	ID     string
	Detail connectors.Detail
}

// connectorDetailErrorMsg carries a FetchDetail failure back to the picker.
type connectorDetailErrorMsg struct {
	ID  string
	Err error
}

// connectorSearchDebounceMsg fires after the manifest-configured debounce
// delay elapses for a remote-mode query change. If Query still matches the
// picker's current input value when received, a real Search call is issued.
type connectorSearchDebounceMsg struct {
	Query string
}

// defaultConnectorSearchDebounce is used when a remote-search manifest does
// not configure a debounce delay.
const defaultConnectorSearchDebounce = 300 * time.Millisecond

// connectorSearchCmd issues a Search call against conn and wraps the result
// into a connectorSearchResultMsg/connectorSearchErrorMsg.
func connectorSearchCmd(conn connectors.Connector, scope, query string) tea.Cmd {
	return func() tea.Msg {
		result, err := conn.Search(context.Background(), connectors.SearchParams{
			Query: query,
			Scope: scope,
		})
		if err != nil {
			return connectorSearchErrorMsg{Query: query, Err: err}
		}
		return connectorSearchResultMsg{Query: query, Items: result.Items}
	}
}

// connectorFetchDetailCmd issues a FetchDetail call for item and wraps the
// result into a connectorDetailResultMsg/connectorDetailErrorMsg.
func connectorFetchDetailCmd(conn connectors.Connector, scope string, item connectors.Item) tea.Cmd {
	return func() tea.Msg {
		detail, err := conn.FetchDetail(context.Background(), connectors.FetchDetailParams{
			ID:    item.ID,
			Scope: scope,
			URI:   item.URI,
		})
		if err != nil {
			return connectorDetailErrorMsg{ID: item.ID, Err: err}
		}
		return connectorDetailResultMsg{ID: item.ID, Detail: detail}
	}
}

// connectorDebounceCmd schedules a connectorSearchDebounceMsg for query
// after delay.
func connectorDebounceCmd(query string, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return connectorSearchDebounceMsg{Query: query}
	})
}
