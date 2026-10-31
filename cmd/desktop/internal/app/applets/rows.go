package applets

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// MaxRows bounds what one refresh hands to the webview. A find over a home
// directory must truncate rather than freeze the window.
const MaxRows = 5000

// Row is one row as the UI and an action's environment see it: every value is
// already text.
type Row map[string]string

// ParseRows turns a data command's stdout into rows. truncated reports that the
// output held more than MaxRows.
func ParseRows(format string, stdout []byte) (rows []Row, truncated bool, err error) {
	if format == FormatLines {
		return parseLines(stdout)
	}
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return []Row{}, false, nil
	}
	if trimmed[0] == '[' {
		return parseArray(trimmed)
	}
	return parseObjectLines(trimmed)
}

func parseLines(stdout []byte) ([]Row, bool, error) {
	rows := []Row{}
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	scanner.Buffer(nil, len(stdout)+1)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(rows) == MaxRows {
			return rows, true, nil
		}
		rows = append(rows, Row{LineKey: line})
	}
	return rows, false, scanner.Err()
}

func parseArray(trimmed []byte) ([]Row, bool, error) {
	var objects []map[string]any
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&objects); err != nil {
		return nil, false, fmt.Errorf("stdout is not a JSON array of objects: %w", err)
	}
	truncated := len(objects) > MaxRows
	if truncated {
		objects = objects[:MaxRows]
	}
	rows := make([]Row, 0, len(objects))
	for _, object := range objects {
		rows = append(rows, rowOf(object))
	}
	return rows, truncated, nil
}

func parseObjectLines(trimmed []byte) ([]Row, bool, error) {
	rows := []Row{}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	for dec.More() {
		if len(rows) == MaxRows {
			return rows, true, nil
		}
		var object map[string]any
		if err := dec.Decode(&object); err != nil {
			return nil, false, fmt.Errorf("stdout row %d is not a JSON object: %w", len(rows)+1, err)
		}
		rows = append(rows, rowOf(object))
	}
	return rows, false, nil
}

func rowOf(object map[string]any) Row {
	row := make(Row, len(object))
	for key, value := range object {
		row[key] = text(value)
	}
	return row
}

func text(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}

var envUnsafe = regexp.MustCompile(`[^A-Z0-9_]`)

// RowEnv is how a row reaches an action's command: HIVE_ROW holds the whole row
// as JSON, and HIVE_ROW_<KEY> holds each field, upper-cased with anything else
// replaced by an underscore. A value in the environment stays one word however
// hostile it is, where one spliced into the command line would need every author
// to remember to quote it.
//
// Keys come back sorted so that when two keys fold to one name the winner does
// not change between runs.
func RowEnv(row Row) ([]string, error) {
	encoded, err := json.Marshal(row)
	if err != nil {
		return nil, errors.New("encoding the row")
	}
	env := []string{"HIVE_ROW=" + string(encoded)}
	for _, key := range slices.Sorted(maps.Keys(row)) {
		name := envUnsafe.ReplaceAllString(strings.ToUpper(key), "_")
		env = append(env, "HIVE_ROW_"+name+"="+row[key])
	}
	return env, nil
}
