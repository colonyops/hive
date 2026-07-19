package flow

import "regexp"

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
