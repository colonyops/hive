// Package rpc implements a custom, minimal JSON-RPC 2.0 protocol over
// newline-delimited stdio for external connector subprocesses. Each method
// call spawns the connector command once, sends exactly one request on
// stdin, reads exactly one response line from stdout, and exits.
package rpc

import "encoding/json"

// Version is the JSON-RPC protocol version used on the wire.
const Version = "2.0"

// Method names supported by the connector RPC protocol.
const (
	MethodInitialize  = "initialize"
	MethodSearch      = "search"
	MethodFetchDetail = "fetchDetail"
)

// Standard JSON-RPC 2.0 error codes (https://www.jsonrpc.org/specification).
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// Request is a single JSON-RPC 2.0 request sent to a connector subprocess on
// stdin.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a single JSON-RPC 2.0 response read from a connector
// subprocess's stdout. Exactly one of Result or Error is set.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface so a JSON-RPC Error can be returned
// and inspected directly by callers via errors.As.
func (e *Error) Error() string {
	return e.Message
}

// InitializeParams carries the scope for the initialize method. Scope is
// optional; connectors that need it must document it.
type InitializeParams struct {
	Scope string `json:"scope,omitempty"`
}

// SearchParams is the wire shape of connectors.SearchParams.
type SearchParams struct {
	Query  string `json:"query,omitempty"`
	Scope  string `json:"scope,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// FetchDetailParams is the wire shape of connectors.FetchDetailParams.
type FetchDetailParams struct {
	ID    string `json:"id"`
	Scope string `json:"scope,omitempty"`
	URI   string `json:"uri,omitempty"`
}

// Manifest is the wire shape of connectors.Manifest, returned by
// initialize.
type Manifest struct {
	ID           string       `json:"id"`
	DisplayName  string       `json:"displayName"`
	Capabilities Capabilities `json:"capabilities"`
	Picker       Picker       `json:"picker"`
}

// Capabilities is the wire shape of connectors.Capabilities.
type Capabilities struct {
	FetchDetail bool `json:"fetchDetail"`
}

// Picker is the wire shape of connectors.PickerManifest.
type Picker struct {
	Layout      string   `json:"layout"`
	Columns     []Column `json:"columns,omitempty"`
	Search      Search   `json:"search"`
	HidePreview bool     `json:"hidePreview,omitempty"`
}

// Column is the wire shape of connectors.Column.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Width int    `json:"width,omitempty"`
	Flex  int    `json:"flex,omitempty"`
}

// Search is the wire shape of connectors.SearchManifest.
type Search struct {
	Mode       string `json:"mode"`
	DebounceMS int    `json:"debounceMs,omitempty"`
}

// SearchResult is the wire shape of connectors.SearchResult, returned by
// search.
type SearchResult struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Item is the wire shape of connectors.Item.
type Item struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle,omitempty"`
	URI      string         `json:"uri,omitempty"`
	Detail   *Detail        `json:"detail,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

// Detail is the wire shape of connectors.Detail, returned by fetchDetail and
// optionally embedded in an Item. At most one of Markdown or KV is set; if
// neither is set the detail kind is "none".
type Detail struct {
	Markdown *MarkdownDetail `json:"markdown,omitempty"`
	KV       *KVDetail       `json:"kv,omitempty"`
}

// MarkdownDetail is the wire shape of connectors.MarkdownDetail.
type MarkdownDetail struct {
	Content string `json:"content"`
}

// KVDetail is the wire shape of connectors.KVDetail.
type KVDetail struct {
	Sections []KVSection `json:"sections"`
}

// KVSection is the wire shape of connectors.KVSection.
type KVSection struct {
	Heading string   `json:"heading,omitempty"`
	Pairs   []KVPair `json:"pairs"`
}

// KVPair is the wire shape of connectors.KVPair.
type KVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
