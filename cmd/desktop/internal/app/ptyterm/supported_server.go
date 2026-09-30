//go:build server

package ptyterm

// The headless server build has no terminal UI to render a PTY into.
const buildSupportsTerminal = false
