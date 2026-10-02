// Command schemagen turns internal/schema/*.cue into the files the binaries
// embed and the site publishes: one JSON Schema per catalog entry, the
// runtime catalog, a copy of each schema under docs/docs/schemas, and a Go
// type file where an entry asks for one. `mise run generate:schemas` runs it
// from the repository root; `check:generate` diffs its outputs.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
)

// entry is one catalog row as catalog.cue declares it.
type entry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Dir         string `json:"dir"`
	Definition  string `json:"definition"`
	GoOut       string `json:"go_out,omitempty"`
	Doc         string `json:"doc"`
}

// runtimeEntry is the projection written to catalog.json: what a binary or
// a listing needs, without the generator configuration.
type runtimeEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Doc         string `json:"doc"`
}

const (
	schemaDir = "internal/schema"
	docsDir   = "docs/docs/schemas"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "schemagen:", err)
		os.Exit(1)
	}
}

func run() error {
	repoRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("cue"); err != nil {
		return fmt.Errorf("cue is not on PATH; run `mise install`")
	}
	root := filepath.Join(repoRoot, schemaDir)

	if err := cue(root, nil, "vet", "./..."); err != nil {
		return err
	}
	entries, err := exportCatalog(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := exportSchema(root, e); err != nil {
			return err
		}
		if err := copyToDocs(root, repoRoot, e); err != nil {
			return err
		}
		if e.GoOut != "" {
			if err := genGo(root, repoRoot, e); err != nil {
				return err
			}
		}
	}
	return nil
}

func exportCatalog(root string) ([]entry, error) {
	var out bytes.Buffer
	if err := cue(root, &out, "export", "-e", "catalog", "--out", "json", "./"); err != nil {
		return nil, err
	}
	var entries []entry
	if err := json.Unmarshal(out.Bytes(), &entries); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	data, err := json.MarshalIndent(project(entries), "", "  ")
	if err != nil {
		return nil, err
	}
	return entries, os.WriteFile(filepath.Join(root, "catalog.json"), append(data, '\n'), 0o644)
}

func project(entries []entry) []runtimeEntry {
	out := make([]runtimeEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, runtimeEntry{ID: e.ID, Title: e.Title, Description: e.Description, Doc: e.Doc})
	}
	return out
}

func schemaPath(root string, e entry) string {
	return filepath.Join(root, e.Dir, e.ID+".schema.json")
}

func exportSchema(root string, e entry) error {
	var out bytes.Buffer
	if err := cue(root, &out, "def", "--out", "jsonschema", "-e", e.Definition, "./"+e.Dir); err != nil {
		return fmt.Errorf("%s: %w", e.ID, err)
	}
	return os.WriteFile(schemaPath(root, e), out.Bytes(), 0o644)
}

func copyToDocs(root, repoRoot string, e entry) error {
	data, err := os.ReadFile(schemaPath(root, e))
	if err != nil {
		return err
	}
	dir := filepath.Join(repoRoot, docsDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, e.ID+".schema.json"), data, 0o644)
}

func genGo(root, repoRoot string, e entry) error {
	target := filepath.Join(repoRoot, e.GoOut)
	if err := cue(root, nil, "exp", "gengotypes", "--outfile", target, "./"+e.Dir); err != nil {
		return fmt.Errorf("%s: %w", e.ID, err)
	}
	src, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	formatted, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("%s: gofmt: %w", e.GoOut, err)
	}
	return os.WriteFile(target, formatted, 0o644)
}

// cue runs the cue binary in dir. stdout goes to out when given; stderr
// always reaches the caller's terminal so a vet failure is readable.
func cue(dir string, out *bytes.Buffer, args ...string) error {
	cmd := exec.Command("cue", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if out != nil {
		cmd.Stdout = out
	} else {
		cmd.Stdout = os.Stdout
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cue %v: %w", args, err)
	}
	return nil
}
