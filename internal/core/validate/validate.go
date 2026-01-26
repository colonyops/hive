// Package validate provides shared validation functions.
package validate

import (
	"fmt"
	"strings"

	"github.com/hay-kot/criterio"
)

// SessionName validates a session name is non-empty after trimming whitespace.
func SessionName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// SessionNameField returns a criterio validator for session names.
func SessionNameField(field, name string) error {
	return criterio.Run(field, name, SessionName)
}
