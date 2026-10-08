// Package diagnostics reads bounded evidence from existing log files.
package diagnostics

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colonyops/hive/pkg/logutils"
)

const (
	MaxTailBytes  = 2 << 20
	MaxEntryBytes = 16 << 10
)

type Entry struct {
	ID        string `json:"id"`
	Time      string `json:"time"`
	Source    string `json:"source"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Raw       string `json:"raw"`
	Truncated bool   `json:"truncated"`
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
	e := Entry{Source: source, Raw: raw, Message: raw, Level: "unknown"}
	var obj struct {
		Time    string `json:"time"`
		Level   string `json:"level"`
		Message string `json:"message"`
		Service string `json:"service_name"`
	}
	if json.Unmarshal([]byte(raw), &obj) == nil && obj.Time != "" {
		e.Source = serviceSource(obj.Service, source)
		e.Time = obj.Time
		e.Level = level(obj.Level)
		e.Message = obj.Message
	} else {
		parts := strings.SplitN(raw, " ", 3)
		if len(parts) == 3 {
			if _, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
				e.Time = parts[0]
				e.Level = level(parts[1])
				e.Message = parts[2]
				for field := range strings.FieldsSeq(parts[2]) {
					if service, ok := strings.CutPrefix(field, logutils.ServiceNameKey+"="); ok {
						e.Source = serviceSource(service, source)
					}
				}
			}
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, e.Time); err == nil {
		e.Time = t.UTC().Format(time.RFC3339Nano)
	} else {
		e.Time = ""
	}
	if len(e.Raw) > MaxEntryBytes {
		e.Raw = e.Raw[:MaxEntryBytes]
		e.Truncated = true
	}
	if len(e.Message) > MaxEntryBytes {
		e.Message = e.Message[:MaxEntryBytes]
		e.Truncated = true
	}
	return e
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
