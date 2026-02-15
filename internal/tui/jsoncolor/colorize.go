package jsoncolor

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/hay-kot/hive/internal/core/styles"
)

// Colorize pretty-prints JSON bytes with theme-aware syntax coloring.
// Returns colorized lines. Falls back to raw string on invalid JSON.
func Colorize(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return string(data)
	}

	var out strings.Builder
	raw := buf.String()

	i := 0
	for i < len(raw) {
		ch := raw[i]
		switch {
		case ch == '"':
			// Find the closing quote, handling escapes
			end := findStringEnd(raw, i)
			str := raw[i : end+1]

			// Check if this is a key (followed by colon)
			rest := strings.TrimLeft(raw[end+1:], " \t")
			if len(rest) > 0 && rest[0] == ':' {
				out.WriteString(styles.TextPrimaryStyle.Render(str))
			} else {
				out.WriteString(styles.TextSuccessStyle.Render(str))
			}
			i = end + 1

		case ch == ':':
			out.WriteString(styles.TextMutedStyle.Render(":"))
			i++

		case ch >= '0' && ch <= '9' || ch == '-':
			end := i + 1
			for end < len(raw) && (raw[end] >= '0' && raw[end] <= '9' || raw[end] == '.' || raw[end] == 'e' || raw[end] == 'E' || raw[end] == '+' || raw[end] == '-') {
				end++
			}
			out.WriteString(styles.TextWarningStyle.Render(raw[i:end]))
			i = end

		case raw[i:] == "true"[:min(4, len(raw)-i)] && i+4 <= len(raw) && raw[i:i+4] == "true":
			out.WriteString(styles.TextSecondaryStyle.Render("true"))
			i += 4

		case raw[i:] == "false"[:min(5, len(raw)-i)] && i+5 <= len(raw) && raw[i:i+5] == "false":
			out.WriteString(styles.TextSecondaryStyle.Render("false"))
			i += 5

		case raw[i:] == "null"[:min(4, len(raw)-i)] && i+4 <= len(raw) && raw[i:i+4] == "null":
			out.WriteString(styles.TextErrorStyle.Render("null"))
			i += 4

		case ch == '{' || ch == '}' || ch == '[' || ch == ']':
			out.WriteString(styles.TextForegroundStyle.Render(string(ch)))
			i++

		default:
			out.WriteByte(ch)
			i++
		}
	}

	return out.String()
}

// findStringEnd returns the index of the closing quote for a JSON string starting at pos.
func findStringEnd(s string, pos int) int {
	for i := pos + 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++ // skip escaped character
			continue
		}
		if s[i] == '"' {
			return i
		}
	}
	return len(s) - 1
}
