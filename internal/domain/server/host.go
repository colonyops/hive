// Package server describes a remote hive server: the ssh target a client
// reaches its daemon through.
package server

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Host struct {
	// Name is an ssh host or config alias, optionally with user@.
	Name string
	// ConfigFile is passed to ssh as -F when set.
	ConfigFile string
	// Binary is the hive path on the remote host. Empty means "hive".
	Binary string
	// Tmux is the tmux path on the remote host. Empty means "tmux", found on the
	// PATH of a non-interactive ssh command, which often lacks a package
	// manager's directory.
	Tmux string
}

var (
	namePattern   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@:-]*$`)
	binaryPattern = regexp.MustCompile(`^[A-Za-z0-9_./~-]+$`)
)

// Validate keeps Name from reading as an ssh option and Binary from carrying
// shell syntax, since ssh hands the remote command to a shell.
func (h Host) Validate() error {
	if !namePattern.MatchString(h.Name) {
		return fmt.Errorf("ssh host %q: use a host or config alias, optionally with user@", h.Name)
	}
	if strings.ContainsAny(h.ConfigFile, "\x00\r\n") {
		return errors.New("ssh config path contains a control character")
	}
	if h.Binary != "" && !binaryPattern.MatchString(h.Binary) {
		return fmt.Errorf("remote hive binary %q: use letters, digits, and _ . / ~ - only", h.Binary)
	}
	if h.Tmux != "" && !binaryPattern.MatchString(h.Tmux) {
		return fmt.Errorf("remote tmux binary %q: use letters, digits, and _ . / ~ - only", h.Tmux)
	}
	return nil
}
