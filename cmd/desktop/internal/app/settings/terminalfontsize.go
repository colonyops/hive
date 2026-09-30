package settings

// useTerminalFont.ts repeats these; these govern the file.
const (
	MinTerminalFontSizePx     = 8
	MaxTerminalFontSizePx     = 64
	DefaultTerminalFontSizePx = 13
)

// Clamps rather than rejects, so one bad field can't block startup.
func TerminalFontSizePx(px int) int {
	if px == 0 {
		return DefaultTerminalFontSizePx
	}
	return min(max(px, MinTerminalFontSizePx), MaxTerminalFontSizePx)
}
