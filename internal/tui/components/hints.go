package components

import (
	"strings"

	"github.com/colonyops/hive/internal/core/styles"
)

// KeyHint renders a single keyboard-shortcut hint: the key in the theme's
// primary color followed by a muted description (e.g. a colored "enter" then a
// muted "select"). It is the building block for footer and help bars so
// shortcut keys stand out from their descriptions.
func KeyHint(key, desc string) string {
	if desc == "" {
		return styles.TextPrimaryStyle.Render(key)
	}
	return styles.TextPrimaryStyle.Render(key) + " " + styles.TextMutedStyle.Render(desc)
}

// KeyHints joins several shortcut hints into a single footer line separated by
// HelpSep. Each entry's key is colored with the theme primary and its
// description muted.
func KeyHints(entries ...HelpEntry) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, KeyHint(e.Key, e.Desc))
	}
	return strings.Join(parts, styles.TextMutedStyle.Render(HelpSep))
}
