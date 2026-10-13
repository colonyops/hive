package drive

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Frontmatter is the metadata a markdown or canvas document carries: a flat
// map for canvases (ValidateFrontmatter), anything YAML allows for markdown.
type Frontmatter map[string]any

// Entry is one row of a space's tree.
type Entry struct {
	Path     string      `json:"path"` // relative to the space root, forward slashes; "" is the root
	Name     string      `json:"name"`
	Kind     Kind        `json:"kind"`
	Size     int64       `json:"size"`
	Modified time.Time   `json:"modified"`
	Created  time.Time   `json:"created"`
	Title    string      `json:"title,omitempty"` // canvas title, or front matter "title"
	Front    Frontmatter `json:"front,omitempty"` // markdown and canvas only
	Children []Entry     `json:"children,omitempty"`
}

var (
	ErrInvalidSpace       = errors.New("drive: invalid space")
	ErrInvalidPath        = errors.New("drive: invalid path")
	ErrNotFound           = errors.New("drive: not found")
	ErrExists             = errors.New("drive: already exists")
	ErrNotEmpty           = errors.New("drive: folder is not empty")
	ErrNotWritable        = errors.New("drive: only markdown and html documents can be written")
	ErrAnchorNotFound     = errors.New("drive: anchor block not found")
	ErrInvalidFrontmatter = errors.New("drive: invalid front matter")
)

// ValidatePath accepts a slash-separated path inside a space: local, no
// empty segment, no segment starting with a dot. The empty path is the space
// root, which no document operation targets, so it is refused too.
func ValidatePath(p string) error {
	if !filepath.IsLocal(p) {
		return fmt.Errorf("%w: %q", ErrInvalidPath, p)
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == "" || seg[0] == '.' || filepath.Base(seg) != seg {
			return fmt.Errorf("%w: %q", ErrInvalidPath, p)
		}
	}
	return nil
}

// SortEntries orders one listing the way a file browser does: folders first,
// then names case-insensitively with digit runs compared as numbers, so
// "2" sorts before "10".
func SortEntries(entries []Entry) {
	slices.SortStableFunc(entries, func(a, b Entry) int {
		if (a.Kind == KindDir) != (b.Kind == KindDir) {
			if a.Kind == KindDir {
				return -1
			}
			return 1
		}
		if c := naturalCompare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})
}

func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			var ad, bd string
			ad, a = digitRun(a)
			bd, b = digitRun(b)
			if c := compareNumbers(ad, bd); c != 0 {
				return c
			}
			continue
		}
		ar, aw := utf8.DecodeRuneInString(a)
		br, bw := utf8.DecodeRuneInString(b)
		if c := compareFolded(ar, br); c != 0 {
			return c
		}
		a, b = a[aw:], b[bw:]
	}
	return len(a) - len(b)
}

func compareFolded(a, b rune) int {
	a, b = unicode.ToLower(a), unicode.ToLower(b)
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareNumbers(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}

func digitRun(s string) (run, rest string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// Find walks root to the entry at path. The empty path is root itself.
func Find(root Entry, path string) (Entry, bool) {
	path = strings.Trim(path, "/")
	if path == "" {
		return root, true
	}
	cur := root
	for seg := range strings.SplitSeq(path, "/") {
		i := slices.IndexFunc(cur.Children, func(e Entry) bool { return e.Name == seg })
		if i < 0 {
			return Entry{}, false
		}
		cur = cur.Children[i]
	}
	return cur, true
}
