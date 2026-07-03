// Command hive-reference-source is a minimal example implementation of
// the hive source JSON-RPC protocol, used by tests and as a protocol
// example. It is invoked once per RPC method call: it reads exactly one
// JSON-RPC request line from stdin, writes exactly one JSON-RPC response
// line to stdout, and exits. It is not intended for end users to discover
// or run standalone.
//
// Diagnostic output is written to stderr only; stdout carries exactly one
// response line so it can be decoded by the source RPC adapter.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/colonyops/hive/internal/sources/rpc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hive-reference-source:", err)
		os.Exit(1)
	}
}

func run() error {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("read request: %w", err)
	}

	var req rpc.Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return writeResponse(rpc.Response{
			JSONRPC: rpc.Version,
			Error: &rpc.Error{
				Code:    rpc.ErrCodeParseError,
				Message: fmt.Sprintf("parse request: %v", err),
			},
		})
	}

	resp := handleRequest(req)
	return writeResponse(resp)
}

// handleRequest dispatches a single JSON-RPC request to the canned
// source implementation below and returns the response to write.
func handleRequest(req rpc.Request) rpc.Response {
	switch req.Method {
	case rpc.MethodInitialize:
		return respondResult(req.ID, canonicalManifest())
	case rpc.MethodSearch:
		return respondResult(req.ID, canonicalSearchResult())
	case rpc.MethodFetchDetail:
		var params rpc.FetchDetailParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return respondError(req.ID, rpc.ErrCodeInvalidParams, fmt.Sprintf("invalid params: %v", err))
		}
		return respondResult(req.ID, canonicalDetail(params.ID))
	default:
		return respondError(req.ID, rpc.ErrCodeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method))
	}
}

// canonicalManifest returns the source's canned picker manifest: a list
// layout with two columns and remote search.
func canonicalManifest() rpc.Manifest {
	return rpc.Manifest{
		ID:          "reference",
		DisplayName: "Reference Source",
		Capabilities: rpc.Capabilities{
			FetchDetail: true,
		},
		Picker: rpc.Picker{
			Layout: "list",
			Columns: []rpc.Column{
				{Key: "title", Label: "Title", Width: 40},
				{Key: "status", Label: "Status", Width: 12},
			},
			Search: rpc.Search{
				Mode:       "remote",
				DebounceMS: 200,
			},
		},
	}
}

// canonicalSearchResult returns three canned items regardless of query.
func canonicalSearchResult() rpc.SearchResult {
	return rpc.SearchResult{
		Items: []rpc.Item{
			{
				ID:       "ref-1",
				Title:    "First reference item",
				Subtitle: "status: open",
				Fields: map[string]any{
					"status": "open",
				},
			},
			{
				ID:       "ref-2",
				Title:    "Second reference item",
				Subtitle: "status: closed",
				Fields: map[string]any{
					"status": "closed",
				},
			},
			{
				ID:       "ref-3",
				Title:    "Third reference item",
				Subtitle: "status: open",
				Fields: map[string]any{
					"status": "open",
				},
			},
		},
	}
}

// canonicalDetail returns markdown detail for known item ids, and an empty
// (DetailKindNone) detail for unknown ids.
func canonicalDetail(id string) rpc.Detail {
	switch id {
	case "ref-1", "ref-2", "ref-3":
		return rpc.Detail{
			Markdown: &rpc.MarkdownDetail{
				Content: fmt.Sprintf("# %s\n\nCanned detail body for item `%s`.", id, id),
			},
		}
	default:
		return rpc.Detail{}
	}
}

func respondResult(id int64, result any) rpc.Response {
	raw, err := json.Marshal(result)
	if err != nil {
		return respondError(id, rpc.ErrCodeInternalError, fmt.Sprintf("encode result: %v", err))
	}
	return rpc.Response{
		JSONRPC: rpc.Version,
		ID:      id,
		Result:  raw,
	}
}

func respondError(id int64, code int, message string) rpc.Response {
	return rpc.Response{
		JSONRPC: rpc.Version,
		ID:      id,
		Error: &rpc.Error{
			Code:    code,
			Message: message,
		},
	}
}

func writeResponse(resp rpc.Response) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	if _, err := os.Stdout.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("write response: %w", err)
	}
	return nil
}
