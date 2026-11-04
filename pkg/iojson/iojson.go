// iojson are utilities for reading and writing JSON IO from a
// command line interface perspective
package iojson

import (
	"encoding/json"
	"io"
)

// WriteLine writes obj as compact JSON (no indent) to w.
// Intended for JSON lines output where multiple objects are written sequentially.
func WriteLine(w io.Writer, obj any) error {
	return json.NewEncoder(w).Encode(obj)
}
