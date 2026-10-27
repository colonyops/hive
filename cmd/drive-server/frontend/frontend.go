// Package frontend embeds the built web UI.
package frontend

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist is the build output rooted at dist. It holds only .gitkeep until the
// Vite build has run.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path and the embed directive guarantees it exists
	}
	return sub
}
