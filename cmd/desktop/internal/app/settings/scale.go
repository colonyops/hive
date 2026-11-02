package settings

// stores/useSurfaceScale.ts repeats these; these govern the file.
const (
	MinInterfaceScale = 75
	MaxInterfaceScale = 150
	MinDocumentScale  = 75
	MaxDocumentScale  = 200
	MinCodeScale      = 75
	MaxCodeScale      = 200
	MinTerminalScale  = 60
	MaxTerminalScale  = 500
	DefaultScale      = 100
)

// ClampScale reads 0 as the default and clamps rather than rejects, so one bad
// field can't block startup.
func ClampScale(percent, low, high int) int {
	if percent == 0 {
		return DefaultScale
	}
	return min(max(percent, low), high)
}
