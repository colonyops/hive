// Package releasenotes embeds the hive CLI's changelog. The parser lives in
// internal/releasenotes; the notes live here because go:embed cannot reach
// above its own directory.
package releasenotes

import (
	"embed"
	"io/fs"
)

// all: keeps unreleased/.gitkeep, so the embed compiles while changelog/ has
// no entry yet and holds the same tree as the worktree.
//
//go:embed all:changelog
var embedded embed.FS

// Changelog is the CLI's changelog directory: one entry per release and the
// unreleased/ fragments.
var Changelog = mustSub(embedded, "changelog")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
