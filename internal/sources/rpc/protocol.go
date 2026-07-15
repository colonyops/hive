// Package rpc implements one-shot JSON-RPC 2.0 source subprocesses using
// newline-delimited messages over stdin and stdout.
package rpc

import "encoding/json"

// Version is the JSON-RPC protocol version used by source subprocesses.
const Version = "2.0"

// Supported JSON-RPC method names.
const (
	MethodInitialize  = "initialize"
	MethodSearch      = "search"
	MethodFetchDetail = "fetchDetail"
)

// Request is one JSON-RPC 2.0 request written to a source subprocess.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// Response is one JSON-RPC 2.0 response read from a source subprocess. Result
// and Error retain their raw presence so the transport can require exactly one.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

// RPCError is the error object returned by a JSON-RPC response.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error returns the remote JSON-RPC error message.
func (e *RPCError) Error() string {
	return e.Message
}

type searchParams struct {
	Query  string `json:"query"`
	Scope  string `json:"scope"`
	Dir    string `json:"dir"`
	Cursor string `json:"cursor"`
}

type fetchDetailParams struct {
	ID    string `json:"id"`
	Scope string `json:"scope"`
	URI   string `json:"uri"`
	Dir   string `json:"dir"`
}
