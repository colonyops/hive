package terminal

import "strings"

// StripANSI removes ANSI escape codes from content.
func StripANSI(content string) string {
	// Fast path: if no escape chars, return as-is. The 8-bit CSI control
	// (U+009B) is the two UTF-8 bytes \xc2\x9b — a lone 0x9B byte is always
	// a UTF-8 continuation byte (e.g. inside a nerd-font glyph), never a
	// real CSI, so we must not treat it as one.
	if !strings.Contains(content, "\x1b") && !strings.Contains(content, "\xc2\x9b") {
		return content
	}

	var b strings.Builder
	b.Grow(len(content))

	i := 0
	for i < len(content) {
		if content[i] == '\x1b' {
			// CSI sequence: ESC [ ... letter
			if i+1 < len(content) && content[i+1] == '[' {
				j := i + 2
				for j < len(content) {
					c := content[j]
					if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
						j++
						break
					}
					j++
				}
				i = j
				continue
			}
			// OSC sequence: ESC ] ... BEL or ST
			if i+1 < len(content) && content[i+1] == ']' {
				bellPos := strings.Index(content[i:], "\x07")
				if bellPos != -1 {
					i += bellPos + 1
					continue
				}
				// Check for ST (ESC \) as alternative terminator
				stPos := strings.Index(content[i:], "\x1b\\")
				if stPos != -1 {
					i += stPos + 2
					continue
				}
			}
			// Other escape: skip 2 chars
			if i+1 < len(content) {
				i += 2
				continue
			}
		}
		// 8-bit CSI: U+009B, encoded in UTF-8 as the two bytes \xc2\x9b.
		if content[i] == '\xc2' && i+1 < len(content) && content[i+1] == '\x9b' {
			j := i + 2
			for j < len(content) {
				c := content[j]
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		b.WriteByte(content[i])
		i++
	}

	return b.String()
}
