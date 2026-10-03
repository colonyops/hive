package dispatch

import sessionsvc "github.com/colonyops/hive/internal/hive/session"

// SessionCreateError is what the app layer reads to log a failed creation and
// hand the form back. Nothing matches on its text (Typed errors, mapped once
// per adapter, architecture.md; ADR a-failed-session-creation-is-a-retryable-draft).
type SessionCreateError = sessionsvc.LaunchError
