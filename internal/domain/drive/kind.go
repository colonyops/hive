package drive

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Kind is what a tree entry is and which renderer and tools apply to it.
//
// ENUM(dir, markdown, html, canvas, file)
type Kind string

// CanvasExt is the canvas file extension. A plain .json is a file: a space
// holds files Hive did not write, so the kind must be readable from the name.
const (
	CanvasExt   = ".canvas.json"
	CanvasesDir = "canvases"
)

const (
	markdownExt   = ".md"
	htmlExt       = ".html"
	maxStemLength = 100
)

// canvasStemPattern is the canvas name rule. Lowercase only because the file
// name is the identity and macOS file systems are case-insensitive: "Report"
// and "report" must not be two canvases that collide on disk.
var canvasStemPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`)

// KindOf classifies a file by its name alone, so a directory is never
// KindOf's answer: the caller knows from the file system that an entry is a
// directory and uses KindDir itself.
func KindOf(name string) Kind {
	name = strings.ToLower(path.Base(name))
	switch {
	case hasExt(name, CanvasExt):
		return KindCanvas
	case hasExt(name, markdownExt):
		return KindMarkdown
	case hasExt(name, htmlExt):
		return KindHtml
	default:
		return KindFile
	}
}

func hasExt(name, ext string) bool {
	return len(name) > len(ext) && strings.HasSuffix(name, ext)
}

// CanvasPath maps the name an agent gives a canvas to its document path. A
// bare name files under canvases/; a name with folders files where it says:
//
//	"perf"                 -> "canvases/perf.canvas.json"
//	"reports/2026-10/perf" -> "reports/2026-10/perf.canvas.json"
//
// A document path that already ends in CanvasExt maps to itself. Folder
// segments follow ValidatePath; the final stem follows the canvas name rule.
func CanvasPath(name string) (string, error) {
	name = strings.TrimSuffix(name, CanvasExt)
	dir, stem := path.Split(name)
	if len(stem) > maxStemLength || !canvasStemPattern.MatchString(stem) {
		return "", fmt.Errorf("%w: canvas name %q", ErrInvalidPath, name)
	}
	if dir == "" {
		return CanvasesDir + "/" + stem + CanvasExt, nil
	}
	dir = strings.TrimSuffix(dir, "/")
	if err := ValidatePath(dir); err != nil {
		return "", err
	}
	return dir + "/" + stem + CanvasExt, nil
}

// CanvasName is the inverse of CanvasPath: the name an agent calls a canvas
// document by. A document directly under canvases/ answers its stem; one
// filed anywhere else answers its path without the extension, which
// CanvasPath maps straight back.
func CanvasName(p string) string {
	p = strings.TrimSuffix(p, CanvasExt)
	if stem, ok := strings.CutPrefix(p, CanvasesDir+"/"); ok && !strings.Contains(stem, "/") {
		return stem
	}
	return p
}
