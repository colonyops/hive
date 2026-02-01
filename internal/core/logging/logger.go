package logging

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Component creates a new logger with a component identifier.
// Uses the "cmp" key for consistency with zerolog conventions.
func Component(name string) zerolog.Logger {
	return log.With().Str("cmp", name).Logger()
}
