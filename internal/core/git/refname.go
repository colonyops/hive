package git

import (
	"fmt"
	"strings"
)

// ValidateBranchName checks whether s is a valid git branch name according to
// git-check-ref-format(1) rules. Returns a descriptive error if invalid.
func ValidateBranchName(s string) error {
	if s == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	if s == "@" {
		return fmt.Errorf("branch name cannot be \"@\"")
	}

	// Disallowed characters (git-check-ref-format rule 3 + rule 4)
	for i, r := range s {
		switch {
		case r <= 0x1f || r == 0x7f:
			return fmt.Errorf("branch name contains control character at position %d", i)
		case r == ' ':
			return fmt.Errorf("branch name cannot contain spaces")
		case r == '~':
			return fmt.Errorf("branch name cannot contain '~'")
		case r == '^':
			return fmt.Errorf("branch name cannot contain '^'")
		case r == ':':
			return fmt.Errorf("branch name cannot contain ':'")
		case r == '?':
			return fmt.Errorf("branch name cannot contain '?'")
		case r == '*':
			return fmt.Errorf("branch name cannot contain '*'")
		case r == '[':
			return fmt.Errorf("branch name cannot contain '['")
		case r == '\\':
			return fmt.Errorf("branch name cannot contain '\\'")
		}
	}

	// Disallowed sequences
	if strings.Contains(s, "..") {
		return fmt.Errorf("branch name cannot contain '..'")
	}
	if strings.Contains(s, "@{") {
		return fmt.Errorf("branch name cannot contain '@{'")
	}
	if strings.Contains(s, "//") {
		return fmt.Errorf("branch name cannot contain '//'")
	}

	// Disallowed starts/ends (whole name)
	if strings.HasPrefix(s, "/") {
		return fmt.Errorf("branch name cannot start with '/'")
	}
	if strings.HasSuffix(s, "/") {
		return fmt.Errorf("branch name cannot end with '/'")
	}
	if strings.HasSuffix(s, ".") {
		return fmt.Errorf("branch name cannot end with '.'")
	}

	// Per-component rules: no component may start with '.' or end with '.lock'
	for _, component := range strings.Split(s, "/") {
		if strings.HasPrefix(component, ".") {
			return fmt.Errorf("branch name component %q cannot start with '.'", component)
		}
		if strings.HasSuffix(component, ".lock") {
			return fmt.Errorf("branch name component %q cannot end with '.lock'", component)
		}
	}

	return nil
}
