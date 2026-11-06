package drive

import (
	"path"
	"strings"
)

// Kind is what a tree entry is and which renderer and tools apply to it.
//
// ENUM(dir, markdown, html, file)
type Kind string

const (
	markdownExt = ".md"
	htmlExt     = ".html"
)

// KindOf classifies a file by its name alone, so a directory is never
// KindOf's answer: the caller knows from the file system that an entry is a
// directory and uses KindDir itself.
func KindOf(name string) Kind {
	name = strings.ToLower(path.Base(name))
	switch {
	case hasExt(name, markdownExt):
		return KindMarkdown
	case hasExt(name, htmlExt):
		return KindHtml
	default:
		return KindFile
	}
}

// hiveStateFiles are what an earlier hive kept beside a space's documents:
// its key-value store and that store's lock. Nothing writes them now, but
// they are still on disk, and they are not documents.
var hiveStateFiles = map[string]struct{}{"kv.json": {}, "kv.json.lock": {}}

// IsHiveState reports whether name is a file hive kept for itself in a space.
// The tree leaves such a file out, as it does a dot entry.
func IsHiveState(name string) bool {
	_, ok := hiveStateFiles[path.Base(name)]
	return ok
}

func hasExt(name, ext string) bool {
	return len(name) > len(ext) && strings.HasSuffix(name, ext)
}
