package flow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/colonyops/hive/cmd/desktop/internal/app/configmigrate"
)

// LoadFlow parses and validates a single flows/*.yaml file. The flow's id is
// the filename stem (extension stripped), never a value read from the file.
// On success it returns the Flow plus any soft warnings; a hard validation
// or parse failure returns a non-nil error and a zero Flow.
func LoadFlow(path string, refs Refs) (Flow, []string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Flow{}, nil, fmt.Errorf("read flow %q: %w", path, err)
	}
	id := flowIDFromFilename(filepath.Base(path))
	data, _, err := configmigrate.FlowSet.Apply(raw)
	if err != nil {
		return Flow{}, nil, fmt.Errorf("flow %q: %w", id, err)
	}
	return parseFlow(id, data, refs)
}

// ParseDocument parses and validates a flow document that is not on disk,
// under the id the caller gives it — a dry run against an unsaved edit, say.
// It is the same migrate-then-decode path LoadFlow uses, so a document that
// parses here is one that would load from a file, and one that does not fails
// with the message the file would have produced.
//
// JSON is accepted too: the decoder is YAML, of which JSON is a subset, so an
// API caller can send the document as a JSON object rather than as text.
func ParseDocument(id string, data []byte, refs Refs) (Flow, []string, error) {
	migrated, _, err := configmigrate.FlowSet.Apply(data)
	if err != nil {
		return Flow{}, nil, fmt.Errorf("flow %q: %w", id, err)
	}
	return parseFlow(id, migrated, refs)
}

// LoadFlows loads every *.yaml/*.yml file directly inside dir, except a
// flow's sibling <id>.ui.yaml layout file (see SaveUI/LoadUI) — layouts
// live in the same directory but are never flow definitions. Each file is
// isolated: a broken flow is recorded in perFileErrors (keyed by filename)
// and skipped, while the remaining files still load. warnings is keyed by
// flow id and only holds entries for flows that produced soft warnings.
func LoadFlows(dir string, refs Refs) (flows []Flow, perFileErrors map[string]error, warnings map[string][]string) {
	perFileErrors = make(map[string]error)
	warnings = make(map[string][]string)

	entries, err := os.ReadDir(dir)
	if err != nil {
		perFileErrors[dir] = fmt.Errorf("read flows dir %q: %w", dir, err)
		return nil, perFileErrors, warnings
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isFlowDefinition(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(dir, name)
		f, warns, err := LoadFlow(path, refs)
		if err != nil {
			perFileErrors[name] = err
			continue
		}
		if len(warns) > 0 {
			warnings[f.ID] = warns
		}
		flows = append(flows, f)
	}
	return flows, perFileErrors, warnings
}

// IsFlowFile reports whether a change to path needs a reload. It keeps
// .ui.yaml, unlike isFlowDefinition, and excludes .sidebar.yaml: the frontend
// owns that state and applies it optimistically, so a reload on its writes
// would flash the sidebar.
func IsFlowFile(path string) bool {
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".sidebar.yaml") || strings.HasSuffix(base, ".sidebar.yml") {
		return false
	}
	ext := filepath.Ext(base)
	return ext == ".yaml" || ext == ".yml"
}

// isFlowDefinition reports whether name is a flow definition file (not a
// .ui.yaml / .sidebar.yaml sibling), factored from LoadFlows.
func isFlowDefinition(name string) bool {
	if strings.HasSuffix(name, ".ui.yaml") || strings.HasSuffix(name, ".ui.yml") {
		return false
	}
	if strings.HasSuffix(name, ".sidebar.yaml") || strings.HasSuffix(name, ".sidebar.yml") {
		return false
	}
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

// flowIDFromFilename strips a .yaml/.yml extension from a base filename to
// derive the flow id. A file with any other (or no) extension keeps its
// full name as the id.
func flowIDFromFilename(name string) string {
	ext := filepath.Ext(name)
	if ext == ".yaml" || ext == ".yml" {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

// parseFlow strictly decodes the flow document, defaults Enabled, checks
// version == configmigrate.FlowSet.Current, and runs validateFlow.
func parseFlow(id string, data []byte, refs Refs) (Flow, []string, error) {
	var file flowFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return Flow{}, nil, fmt.Errorf("flow %q: %w", id, err)
	}

	if file.Version != configmigrate.FlowSet.Current {
		return Flow{}, nil, fmt.Errorf("flow %q: version must be %d, got %d", id, configmigrate.FlowSet.Current, file.Version)
	}

	enabled := true
	if file.Enabled != nil {
		enabled = *file.Enabled
	}

	resurface := file.Resurface
	if resurface == "" {
		resurface = DefaultResurfacePolicy
	}
	f := Flow{
		ID: id, Name: file.Name, Enabled: enabled, Resurface: resurface,
		Image: file.Image, Nodes: file.Nodes, Wires: file.Wires,
	}

	warnings, err := validateFlow(&f, refs)
	if err != nil {
		return Flow{}, nil, fmt.Errorf("flow %q: %w", id, err)
	}
	return f, warnings, nil
}
