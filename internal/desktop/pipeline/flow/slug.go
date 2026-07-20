package flow

import (
	"regexp"
	"strings"
)

// maxSlugLen bounds every id/ref in the flow schema (node ids, and the
// source/feed/action ids nodes reference).
const maxSlugLen = 64

// slugPattern matches lowercase kebab-case identifiers: a leading letter or
// digit, then any run of lowercase letters, digits, or hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validSlug reports whether s is a valid id/ref per the flow schema's slug
// rule: `^[a-z0-9][a-z0-9-]*$`, at most maxSlugLen characters.
func validSlug(s string) bool {
	return s != "" && len(s) <= maxSlugLen && slugPattern.MatchString(s)
}

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugify coerces an arbitrary display name into a valid slug: lowercase,
// non-alphanumeric runs become single hyphens, leading/trailing hyphens are
// trimmed, capped at maxSlugLen. Returns "flow" for input that reduces to
// empty, so a new flow always has a usable id.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlugRun.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > maxSlugLen {
		s = strings.Trim(s[:maxSlugLen], "-")
	}
	if s == "" {
		return "flow"
	}
	return s
}
