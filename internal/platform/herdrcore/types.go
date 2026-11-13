package herdrcore

import (
	"encoding/json"
	"fmt"
)

// Generation is the herdr-core protocol generation this client speaks.
const Generation = 1

// Terminal is the subset of herdr-core's terminal record hive reads.
type Terminal struct {
	Address  string            `json:"address"`
	Name     *string           `json:"name,omitempty"`
	Argv     []string          `json:"argv,omitempty"`
	Cwd      string            `json:"cwd"`
	State    string            `json:"state"`
	Status   *string           `json:"status,omitempty"`
	Title    *string           `json:"title,omitempty"`
	Cols     uint16            `json:"cols"`
	Rows     uint16            `json:"rows"`
	Keep     bool              `json:"keep"`
	Labels   map[string]string `json:"labels,omitempty"`
	ExitCode *int32            `json:"exit_code,omitempty"`
	Reason   *string           `json:"reason,omitempty"`
}

// Display is what a viewer looks like: colors as `#rrggbb` and the cell size
// in pixels. The core answers the program's display queries with it.
type Display struct {
	FG     *string    `json:"fg,omitempty"`
	BG     *string    `json:"bg,omitempty"`
	Cursor *string    `json:"cursor,omitempty"`
	Cell   *[2]uint32 `json:"cell,omitempty"`
}

// Size is a terminal size in cells.
type Size struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// RunParams are terminal.run's params. With neither Argv nor Line the core
// starts the machine's shell.
type RunParams struct {
	Argv     []string          `json:"argv,omitempty"`
	Line     string            `json:"line,omitempty"`
	Name     string            `json:"name,omitempty"`
	Cwd      string            `json:"cwd,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Keep     bool              `json:"keep,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	Size     *Size             `json:"size,omitempty"`
	Display  *Display          `json:"display,omitempty"`
	NoParent bool              `json:"no_parent,omitempty"`
}

// AttachParams are terminal.attach's params.
type AttachParams struct {
	Terminal string   `json:"terminal"`
	Cols     uint16   `json:"cols"`
	Rows     uint16   `json:"rows"`
	History  int      `json:"history,omitempty"`
	Observe  bool     `json:"observe,omitempty"`
	Display  *Display `json:"display,omitempty"`
}

// AttachResult is terminal.attach's result.
type AttachResult struct {
	Channel        uint32   `json:"channel"`
	Terminal       Terminal `json:"terminal"`
	AutomaticInput bool     `json:"automatic_input"`
}

// CloseReason is what the core says when it ends an attach stream.
type CloseReason struct {
	Reason   string `json:"reason"`
	ExitCode *int32 `json:"exit_code"`
	Signal   *int32 `json:"signal"`
}

// HelloResult is the subset of hello's result hive reads.
type HelloResult struct {
	Generation uint32   `json:"generation"`
	Version    string   `json:"version"`
	Features   []string `json:"features"`
	Methods    []string `json:"methods"`
}

// RPCError is a JSON-RPC error from the core. Kind is the stable error kind
// (`not_found`, `finished`, `unavailable`, ...).
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Kind string `json:"kind"`
		Hint string `json:"hint"`
	} `json:"data"`
}

func (e *RPCError) Error() string {
	if e.Data.Hint != "" {
		return fmt.Sprintf("herdr-core: %s (%s)", e.Message, e.Data.Hint)
	}
	return "herdr-core: " + e.Message
}

// Kind returns the stable error kind of err when it is an RPCError.
func (e *RPCError) Kind() string { return e.Data.Kind }

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcMessage struct {
	ID     *int64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
}
