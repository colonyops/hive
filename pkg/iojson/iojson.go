// iojson are utilities for reading and writing JSON IO from a
// command line interface perspective
package iojson

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

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

func WriteWith(w io.Writer, ew io.Writer, obj any) error {
	bits, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		errStr := jsonError("error marshaling in iojson.Write", err)
		_, err = fmt.Fprintln(ew, errStr)
		return err
	}

	_, err = fmt.Fprintln(w, string(bits))
	return err
}

// Write calls WriteWith with [os.Stdout] and [os.Stderr]
func Write(obj any) error {
	return WriteWith(os.Stdout, os.Stderr, obj)
}
