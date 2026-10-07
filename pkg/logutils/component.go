package logutils

import "github.com/rs/zerolog"

const (
	// ComponentKey names the part of a program that emitted a log line.
	ComponentKey = "cmp"
	// ServiceNameKey distinguishes programs that write to the shared log file.
	ServiceNameKey = "service_name"

	// ServiceNameCLI identifies the hive CLI.
	ServiceNameCLI = "hive-cli"
	// ServiceNameDesktop identifies Hive Desktop.
	ServiceNameDesktop = "hive-desktop"
)

// Component returns logger with the component label set to name. Give it a
// logger that has no component label yet: zerolog appends fields, so a second
// call adds a second cmp key instead of replacing the first.
func Component(logger zerolog.Logger, name string) zerolog.Logger {
	return logger.With().Str(ComponentKey, name).Logger()
}

// Service returns logger with the program identity set to name.
func Service(logger zerolog.Logger, name string) zerolog.Logger {
	return logger.With().Str(ServiceNameKey, name).Logger()
}
