// Package multiplexer defines implementation-neutral terminal multiplexer values.
package multiplexer

import (
	"fmt"
	"strings"
)

// Target identifies a session, window, or pane.
type Target struct {
	Session string
	Window  string
	Pane    string
}

// ValidateSession validates a session target.
func (t Target) ValidateSession() error {
	if err := validatePart("session", t.Session); err != nil {
		return err
	}
	if t.Window != "" || t.Pane != "" {
		return fmt.Errorf("session target must not include a window or pane")
	}
	return nil
}

// ValidateWindow validates a qualified window target.
func (t Target) ValidateWindow() error {
	if err := validatePart("session", t.Session); err != nil {
		return err
	}
	if err := validatePart("window", t.Window); err != nil {
		return err
	}
	if t.Pane != "" {
		return fmt.Errorf("window target must not include a pane")
	}
	return nil
}

// ValidatePane validates a qualified pane target.
func (t Target) ValidatePane() error {
	if err := validatePart("session", t.Session); err != nil {
		return err
	}
	if err := validatePart("window", t.Window); err != nil {
		return err
	}
	return validatePart("pane", t.Pane)
}

func validatePart(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s target is required", name)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s target contains NUL", name)
	}
	return nil
}
