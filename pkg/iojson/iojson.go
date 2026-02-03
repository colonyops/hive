// iojson are utilities for reading and writing JSON IO from a
// command line interface perspective
package iojson

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Options configures JSON output behavior.
type Options struct {
	Indent bool
}

// Option is a functional option for configuring JSON output.
type Option func(*Options)

// WithIndent sets whether to indent JSON output (default: true).
func WithIndent(indent bool) Option {
	return func(o *Options) {
		o.Indent = indent
	}
}

func defaultOptions() Options {
	return Options{Indent: true}
}

// Error is the standard error format type that is returned when errors
// happen.
type Error struct {
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

func jsonError(msg string, jsonErr error) string {
	// Use json.Marshal to properly escape strings
	msgBytes, _ := json.Marshal(msg)
	errBytes, _ := json.Marshal(jsonErr.Error())
	return fmt.Sprintf(`{"message":%s,"data":{"json_error":%s}}`, msgBytes, errBytes)
}

// MarshalError is a utility for creating an error struct, it will first
// attempt the marshal the struct, if that fails it will return return
// a manually constructed JSON blob with the provided error msg and
// a note that there was a marashling error and the this indicates a
// bug in the software.
func MarshalError(msg string, data map[string]any) string {
	resp := Error{Message: msg, Data: data}

	bits, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return jsonError(msg, err)
	}

	return string(bits)
}

func WriteError(str string, data map[string]any) error {
	errstr := MarshalError(str, data)

	_, err := fmt.Fprintln(os.Stderr, errstr)
	return err
}

// WriteWith writes obj as JSON to w, with errors written to ew.
func WriteWith(w io.Writer, ew io.Writer, obj any, opts ...Option) error {
	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	var bits []byte
	var err error
	if options.Indent {
		bits, err = json.MarshalIndent(obj, "", "  ")
	} else {
		bits, err = json.Marshal(obj)
	}
	if err != nil {
		errStr := jsonError("error marshaling in iojson.Write", err)
		_, err = fmt.Fprintln(ew, errStr)
		return err
	}

	_, err = fmt.Fprintln(w, string(bits))
	return err
}

// Write calls WriteWith with [os.Stdout] and [os.Stderr].
func Write(obj any, opts ...Option) error {
	return WriteWith(os.Stdout, os.Stderr, obj, opts...)
}

// WriteLine writes obj as compact JSON (no indent) to w.
// Intended for JSON lines output where multiple objects are written sequentially.
func WriteLine(w io.Writer, obj any) error {
	return json.NewEncoder(w).Encode(obj)
}
