package applets

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

//go:embed examples/*/applet.yaml
var examples embed.FS

// Example is one complete applet shipped as documentation.
type Example struct {
	ID   string
	YAML string
}

// Examples returns the shipped examples. A test parses every one, so the prompt
// an agent reads cannot teach a schema the loader refuses.
func Examples() []Example {
	dirs, err := fs.ReadDir(examples, "examples")
	if err != nil {
		panic(err)
	}
	out := make([]Example, 0, len(dirs))
	for _, dir := range dirs {
		raw, err := examples.ReadFile(path.Join("examples", dir.Name(), FileName))
		if err != nil {
			panic(err)
		}
		out = append(out, Example{ID: dir.Name(), YAML: strings.TrimSpace(string(raw))})
	}
	return out
}
