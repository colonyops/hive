package flow

import (
	"regexp"
	"strings"

	"github.com/colonyops/hive/cmd/desktop/internal/app/slug"
)

var nonSlugRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugify coerces an arbitrary display name into a valid slug: lowercase,
// non-alphanumeric runs become single hyphens, leading/trailing hyphens are
// trimmed, capped at slug.MaxLen. Returns "flow" for input that reduces to
// empty, so a new flow always has a usable id.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlugRun.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > slug.MaxLen {
		s = strings.Trim(s[:slug.MaxLen], "-")
	}
	if s == "" {
		return "flow"
	}
	return s
}
