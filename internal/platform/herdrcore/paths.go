// Package herdrcore is a client of herdr-core (`hc`), the terminal runtime
// that keeps programs alive in terminals and relays their raw bytes to
// viewers. It speaks the core's JSON-RPC and framed attach protocol
// (schema/README.md in herdrdev/herdr-core).
package herdrcore

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// SocketPath is where the core for env's HC_SESSION listens on Unix,
// following herdr-core's paths.rs: HC_SOCKET, else
// $XDG_RUNTIME_DIR/herdr-core/<session>.sock, else beside the state in
// $XDG_STATE_HOME (default ~/.local/state)/herdr-core/<session>/. The
// `socket` override in herdr-core's config.toml is not read.
func SocketPath(getenv func(string) string) (string, error) {
	if s := getenv("HC_SOCKET"); s != "" {
		return s, nil
	}
	session := strings.TrimSpace(getenv("HC_SESSION"))
	if session == "" {
		session = "default"
	}
	if session == "." || session == ".." || strings.ContainsAny(session, `/\:*?"<>|`) {
		return "", errors.New("herdrcore: HC_SESSION can't be a folder name")
	}
	if run := getenv("XDG_RUNTIME_DIR"); run != "" {
		return filepath.Join(run, "herdr-core", session+".sock"), nil
	}
	if run := filepath.Join("/run/user", strconv.Itoa(os.Getuid())); isDir(run) {
		return filepath.Join(run, "herdr-core", session+".sock"), nil
	}
	state := getenv("XDG_STATE_HOME")
	if state == "" {
		home := getenv("HOME")
		if home == "" {
			home = "/"
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "herdr-core", session, session+".sock"), nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
