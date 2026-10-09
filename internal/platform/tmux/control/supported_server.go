//go:build server && !terminals

package tmuxcc

// The headless server build has no terminal UI to attach to.
const buildSupportsTerminal = false
