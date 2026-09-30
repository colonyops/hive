//go:build !server

package ptyterm

// buildSupportsTerminal gates the subsystem. The PTY path compiles on every
// unix, so this constant plus a GOOS check is the entire native/stub fork.
const buildSupportsTerminal = true
