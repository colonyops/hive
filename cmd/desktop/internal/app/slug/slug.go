// Package slug is the id rule the flow and action schemas share.
package slug

import "regexp"

// MaxLen bounds every slug: flow node ids, the source/feed/action ids nodes
// reference, and action and launcher ids.
const MaxLen = 64

var pattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Valid reports whether s is lowercase kebab-case, `^[a-z0-9][a-z0-9-]*$`, and
// at most MaxLen characters.
func Valid(s string) bool {
	return s != "" && len(s) <= MaxLen && pattern.MatchString(s)
}
