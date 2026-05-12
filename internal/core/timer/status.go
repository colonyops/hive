// Package timer holds shared types and helpers used by the hive timer
// command. The Status enum is referenced by the timers table sqlc override.
package timer

// Status tracks the lifecycle of a scheduled timer row.
//
// ENUM(
//
//	active
//	fired
//	failed
//	orphaned
//
// )
type Status string
