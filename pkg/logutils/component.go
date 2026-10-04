package logutils

import "github.com/rs/zerolog"

// ComponentKey is the field that names the part of the program a log line
// comes from. Every component logger carries it, so one filter narrows a log
// to one component.
const ComponentKey = "cmp"

// Component returns logger with the component label set to name. Give it a
// logger that has no component label yet: zerolog appends fields, so a second
// call adds a second cmp key instead of replacing the first.
func Component(logger zerolog.Logger, name string) zerolog.Logger {
	return logger.With().Str(ComponentKey, name).Logger()
}
