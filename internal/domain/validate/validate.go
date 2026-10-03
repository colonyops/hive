// Package validate provides shared validation functions.
package validate

import "fmt"

// SessionID validates a session ID follows the expected format:
// - Non-empty
// - Lowercase alphanumeric only (a-z, 0-9)
// - No spaces or special characters
func SessionID(id string) error {
	if id == "" {
		return fmt.Errorf("session ID is required")
	}
	for _, r := range id {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if !isLower && !isDigit {
			return fmt.Errorf("session ID must be lowercase alphanumeric only, got %q", id)
		}
	}
	return nil
}
