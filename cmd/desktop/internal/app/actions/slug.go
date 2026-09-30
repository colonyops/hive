package actions

import "regexp"

// maxSlugLen bounds every action id in the schema, mirroring flow's
// maxSlugLen for node/source/feed/action ids.
const maxSlugLen = 64

// slugPattern matches lowercase kebab-case identifiers: a leading letter or
// digit, then any run of lowercase letters, digits, or hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validSlug reports whether s is a valid action id per the shared schema
// slug rule: `^[a-z0-9][a-z0-9-]*$`, at most maxSlugLen characters.
func validSlug(s string) bool {
	return s != "" && len(s) <= maxSlugLen && slugPattern.MatchString(s)
}
