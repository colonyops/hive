//go:build !server

package tmuxcc

// buildSupportsTerminal gates the whole subsystem. The os/exec attach path
// compiles everywhere, so this constant plus a GOOS check is the entire
// native/stub fork.
const buildSupportsTerminal = true
