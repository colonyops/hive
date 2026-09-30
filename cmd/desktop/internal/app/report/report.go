// Package report builds the two halves of a bug report, which are deliberately
// not one artifact. IssueURL is public and carries only the build identity.
// Assemble builds a redacted bundle that must never reach a public issue: the
// user sends it to a maintainer privately, when one asks.
//
// Redaction is why they are separate. It removes credentials, not identity. A
// log tail still names the user's home directory, repositories and branches,
// and flows still name their orgs and hosts (ADR
// problem-reports-are-github-issues).
package report

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/settings"
)

// MaxBundleBytes is GitHub's per-attachment ceiling for a non-image file. A
// bundle over it cannot be attached to the issue it exists for.
const MaxBundleBytes = 25 * 1024 * 1024

const maxLogTailBytes = 256 * 1024

// Build is the ldflags-stamped identity of the running binary, supplied by the
// adapter because -X binds to package main.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// Options chooses what the bundle carries beyond build info, which is always
// present. Every surface here is opt-in and its zero value is deliberate.
type Options struct {
	Channel string

	IncludeLogs     bool
	IncludeSettings bool
	IncludeFlows    bool
	IncludeActions  bool
}

type Bundle struct {
	ReportID    string         `json:"report_id"`
	GeneratedAt time.Time      `json:"generated_at"`
	Build       BuildInfo      `json:"build"`
	Logs        *LogTail       `json:"logs,omitempty"`
	Config      ConfigSnapshot `json:"config"`
}

// Inventory is what the reporter could attach, read from disk, so the dialog
// can label each toggle without the bundle itself.
type Inventory struct {
	HasSettings bool
	FlowCount   int
	HasActions  bool
	HasLogs     bool
	LogBytes    int
}

type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	Channel   string `json:"channel,omitempty"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version"`
	NumCPU    int    `json:"num_cpu"`
}

type LogTail struct {
	Path      string `json:"path"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated"`
	Content   string `json:"content"`
}

type ConfigSnapshot struct {
	Settings any        `json:"settings,omitempty"`
	Flows    []FlowFile `json:"flows,omitempty"`
	Actions  any        `json:"actions,omitempty"`
}

type FlowFile struct {
	Name    string `json:"name"`
	Content any    `json:"content"`
}

type Assembler struct {
	paths settings.Paths
	build Build
}

func NewAssembler(paths settings.Paths, build Build) *Assembler {
	return &Assembler{paths: paths, build: build}
}

func (a *Assembler) Assemble(id string, at time.Time, opts Options) *Bundle {
	b := &Bundle{
		ReportID:    id,
		GeneratedAt: at,
		Build:       a.BuildInfo(opts.Channel),
	}
	if opts.IncludeLogs {
		b.Logs = readLogTail(a.paths.LogFile, maxLogTailBytes)
	}
	if opts.IncludeSettings {
		b.Config.Settings = redactedYAML(a.paths.SettingsPath)
	}
	if opts.IncludeFlows {
		b.Config.Flows = redactedFlows(a.paths.FlowsDir)
	}
	if opts.IncludeActions {
		b.Config.Actions = redactedYAML(a.paths.ActionsPath)
	}
	return b
}

// Inventory reads every attachable surface and reports what is present, so the
// dialog can show accurate counts and hide toggles for what does not exist.
func (a *Assembler) Inventory() Inventory {
	inv := Inventory{
		HasSettings: redactedYAML(a.paths.SettingsPath) != nil,
		HasActions:  redactedYAML(a.paths.ActionsPath) != nil,
		FlowCount:   len(redactedFlows(a.paths.FlowsDir)),
	}
	if tail := readLogTail(a.paths.LogFile, maxLogTailBytes); tail != nil {
		inv.HasLogs = true
		inv.LogBytes = tail.Bytes
	}
	return inv
}

// BuildInfo is exported because the public half of a report needs it without
// assembling a bundle.
func (a *Assembler) BuildInfo(channel string) BuildInfo {
	return BuildInfo{
		Version:   a.build.Version,
		Commit:    a.build.Commit,
		Date:      a.build.Date,
		Channel:   channel,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		NumCPU:    runtime.NumCPU(),
	}
}

func GzipJSON(b *Bundle) ([]byte, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// readLogTail returns the last max bytes of the log with a partial leading
// line dropped, and secret-looking values scrubbed. A missing log is nil.
func readLogTail(path string, max int64) *LogTail {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil
	}

	start, truncated := int64(0), false
	if info.Size() > max {
		start, truncated = info.Size()-max, true
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil
	}

	content := string(data)
	if truncated {
		if i := strings.IndexByte(content, '\n'); i >= 0 {
			content = content[i+1:]
		}
	}
	content = scrubText(content)
	return &LogTail{Path: path, Bytes: len(content), Truncated: truncated, Content: content}
}

func redactedYAML(path string) any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	return Redact(doc)
}

func redactedFlows(dir string) []FlowFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var flows []FlowFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		if strings.HasSuffix(name, ".ui.yaml") || strings.HasSuffix(name, ".sidebar.yaml") {
			continue
		}
		if content := redactedYAML(filepath.Join(dir, name)); content != nil {
			flows = append(flows, FlowFile{Name: name, Content: content})
		}
	}
	return flows
}
