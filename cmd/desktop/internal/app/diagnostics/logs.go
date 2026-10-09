package diagnostics

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/colonyops/hive/pkg/logutils"
)

const (
	MaxTailBytes       = 2 << 20
	MaxEntryBytes      = 16 << 10
	maxFields          = 64
	maxFieldValueBytes = 4 << 10
)

type Entry struct {
	ID        string            `json:"id"`
	Time      string            `json:"time"`
	Source    string            `json:"source"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields"`
	Raw       string            `json:"raw"`
	Truncated bool              `json:"truncated"`
}

type Source struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	UpdatedAt string `json:"updatedAt"`
	Error     string `json:"error"`
	Truncated bool   `json:"truncated"`
}

func CLIPath(environ []string, fallback string) string {
	env := make(map[string]string)
	for _, item := range environ {
		if k, v, ok := strings.Cut(item, "="); ok {
			env[k] = v
		}
	}
	if path := env["HIVE_LOG_FILE"]; path != "" {
		return path
	}
	if dir := env["HIVE_DATA_DIR"]; dir != "" {
		return filepath.Join(dir, "hive.log")
	}
	if dir := env["XDG_DATA_HOME"]; dir != "" {
		return filepath.Join(dir, "hive", "hive.log")
	}
	return filepath.Join(fallback, "hive.log")
}

func Read(id, path string) (Source, []Entry) {
	source := Source{ID: id, Path: path}
	entries := make([]Entry, 0)
	info, err := os.Stat(path)
	if err != nil {
		source.Error = err.Error()
		return source, entries
	}
	if !info.Mode().IsRegular() {
		source.Error = "Not a regular log file"
		return source, entries
	}
	f, err := os.Open(path)
	if err != nil {
		source.Error = err.Error()
		return source, entries
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil {
		source.Error = err.Error()
		return source, entries
	}
	if !info.Mode().IsRegular() {
		source.Error = "Not a regular log file"
		return source, entries
	}
	source.UpdatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	start := max(int64(0), info.Size()-MaxTailBytes)
	data, err := io.ReadAll(io.NewSectionReader(f, start, MaxTailBytes))
	if err != nil {
		source.Error = err.Error()
		return source, entries
	}
	source.Truncated = start > 0
	if start > 0 {
		if n := bytes.IndexByte(data, '\n'); n >= 0 {
			data = data[n+1:]
			start += int64(n + 1)
		} else {
			return source, entries
		}
	}
	for len(data) > 0 {
		n := bytes.IndexByte(data, '\n')
		if n < 0 {
			break
		} // A writer may still be appending this record.
		raw := string(data[:n])
		data = data[n+1:]
		offset := start
		start += int64(n + 1)
		if raw == "" {
			continue
		}
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", path, offset, raw)))
		entry := Parse(id, raw)
		entry.ID = fmt.Sprintf("%s-%x", entry.Source, sum[:10])
		entries = append(entries, entry)
	}
	return source, entries
}

func Parse(source, raw string) Entry {
	e := Entry{Source: source, Raw: raw, Message: raw, Level: "unknown", Fields: make(map[string]string)}
	if obj, ok := decodeJSONObject(raw); ok {
		e.Time = stringValue(obj["time"])
		e.Level = level(stringValue(obj["level"]))
		if message := stringValue(obj["message"]); message != "" {
			e.Message = message
		}
		e.Source = serviceSource(stringValue(obj[logutils.ServiceNameKey]), source)
		keys := make([]string, 0, len(obj))
		for key := range obj {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if key == "time" || key == "level" || key == "message" {
				continue
			}
			if addField(e.Fields, key, valueString(obj[key])) {
				e.Truncated = true
			}
		}
	} else {
		parts := strings.SplitN(raw, " ", 3)
		if len(parts) == 3 {
			if _, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
				e.Time = parts[0]
				e.Level = level(parts[1])
				e.Message = parts[2]
				for field := range strings.FieldsSeq(parts[2]) {
					key, value, ok := strings.Cut(field, "=")
					if !ok || key == "" {
						continue
					}
					if addField(e.Fields, key, value) {
						e.Truncated = true
					}
				}
				e.Source = serviceSource(e.Fields[logutils.ServiceNameKey], source)
			}
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, e.Time); err == nil {
		e.Time = t.UTC().Format(time.RFC3339Nano)
	} else {
		e.Time = ""
	}
	if raw, truncated := truncateUTF8(e.Raw, MaxEntryBytes); truncated {
		e.Raw = raw
		e.Truncated = true
	}
	if message, truncated := truncateUTF8(e.Message, MaxEntryBytes); truncated {
		e.Message = message
		e.Truncated = true
	}
	return e
}

func decodeJSONObject(raw string) (map[string]any, bool) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var obj map[string]any
	if err := decoder.Decode(&obj); err != nil {
		return nil, false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, false
	}
	return obj, true
}

// NewEntry normalizes a record supplied by a non-file diagnostics source.
func NewEntry(source, id, at, severity, message, raw string, fields map[string]string) Entry {
	e := Entry{ID: id, Time: at, Source: source, Level: level(severity), Message: message, Fields: make(map[string]string), Raw: raw}
	for key, value := range fields {
		if addField(e.Fields, key, value) {
			e.Truncated = true
		}
	}
	if raw, truncated := truncateUTF8(e.Raw, MaxEntryBytes); truncated {
		e.Raw = raw
		e.Truncated = true
	}
	if text, truncated := truncateUTF8(e.Message, MaxEntryBytes); truncated {
		e.Message = text
		e.Truncated = true
	}
	return e
}

func addField(fields map[string]string, key, value string) bool {
	_, exists := fields[key]
	if (!exists && len(fields) >= maxFields) || key == "" || len(key) > 256 {
		return true
	}
	var truncated bool
	fields[key], truncated = truncateUTF8(value, maxFieldValueBytes)
	return truncated
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func valueString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func truncateUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value, true
}

func level(s string) string {
	switch strings.ToLower(s) {
	case "inf", "info":
		return "info"
	case "wrn", "warn":
		return "warn"
	case "err", "error", "fatal", "ftl", "panic", "pnc":
		return "error"
	case "dbg", "debug", "trace", "trc":
		return "debug"
	default:
		return "unknown"
	}
}

func serviceSource(service, fallback string) string {
	switch service {
	case logutils.ServiceNameCLI:
		return "cli"
	case logutils.ServiceNameDesktop:
		return "desktop"
	default:
		return fallback
	}
}
