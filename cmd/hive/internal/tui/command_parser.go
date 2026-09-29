package tui

import "strings"

// ParsedCommand represents a parsed command input.
type ParsedCommand struct {
	Name string
	Args []string
}

// ParseCommandInput parses a command string like ":command arg1 arg2" into name and args.
// The input should start with ':' but it's optional.
// Arguments are split by whitespace.
func ParseCommandInput(input string) ParsedCommand {
	// Trim leading/trailing whitespace
	input = strings.TrimSpace(input)

	// Remove leading ':' if present
	input = strings.TrimPrefix(input, ":")

	// Split by whitespace
	parts := strings.Fields(input)

	if len(parts) == 0 {
		return ParsedCommand{}
	}

	return ParsedCommand{
		Name: parts[0],
		Args: parts[1:],
	}
}
