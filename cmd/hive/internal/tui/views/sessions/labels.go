package sessions

import "strings"

// FormatLabels renders labels for a session row or header: "auto · backend".
func FormatLabels(labels []string) string {
	return strings.Join(labels, " · ")
}
