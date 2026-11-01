package applets

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
)

//go:embed builtin
var builtins embed.FS

// BuilderID is the built-in applet that builds an authoring prompt.
const BuilderID = "applet-builder"

// ReadBuiltin returns one file of a built-in applet.
func ReadBuiltin(id, name string) ([]byte, error) {
	return builtins.ReadFile(path.Join("builtin", id, filepath.ToSlash(name)))
}

// loadBuiltins parses what ships in the binary. A file that does not parse is a bug in
// this package, which TestBuiltinsParse catches before it can ship.
func loadBuiltins(root string) map[string]Entry {
	dirs, err := fs.ReadDir(builtins, "builtin")
	if err != nil {
		panic(err)
	}
	out := make(map[string]Entry, len(dirs))
	for _, dir := range dirs {
		id := dir.Name()
		raw, err := ReadBuiltin(id, FileName)
		if err != nil {
			panic(err)
		}
		applet, err := Parse(id, raw, "")
		if err != nil {
			panic(fmt.Errorf("built-in applet %q: %w", id, err))
		}
		applet.Dir = filepath.Join(root, id)
		applet.Builtin = true
		out[id] = Entry{Applet: applet}
	}
	return out
}
