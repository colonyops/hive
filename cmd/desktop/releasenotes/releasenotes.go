// Package releasenotes embeds Hive Desktop's changelog. The parser lives in
// internal/releasenotes; the notes live here because go:embed cannot reach
// above its own directory.
package releasenotes

import (
	"embed"
	"io/fs"
	"sync"

	"github.com/colonyops/hive/internal/releasenotes"
)

//go:embed all:changelog
var embedded embed.FS

// Changelog is the desktop's changelog directory: one entry per release and
// the unreleased/ fragments.
var Changelog = mustSub(embedded, "changelog")

// Load parses the embedded changelog once per process.
var Load = sync.OnceValues(func() (releasenotes.Entries, error) {
	return releasenotes.Load(Changelog)
})

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
