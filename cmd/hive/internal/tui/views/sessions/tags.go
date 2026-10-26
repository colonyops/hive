package sessions

import "strings"

// FormatTags renders tags for a session row or header: "auto · backend".
func FormatTags(tags []string) string {
	return strings.Join(tags, " · ")
}
