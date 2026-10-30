//go:build server && !serverterminal

package tmuxcc

// The headless server build has no terminal UI to attach to.
const buildSupportsTerminal = false
