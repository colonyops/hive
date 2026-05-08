package lua

import (
	glua "github.com/yuin/gopher-lua"
)

// HostModule registers part of the `hive` host API on the Lua state. Each
// module owns its own state and the field(s) it attaches to the hive table;
// adding a new API surface is "implement Register and pass an instance to
// NewRuntime."
type HostModule interface {
	Register(state *glua.LState, hive *glua.LTable) error
}

// HostModuleCloser is an optional add-on for HostModules with background
// resources. Plugin.Close invokes it before closing the LState so workers
// stop while Lua callbacks are still safe to invoke.
type HostModuleCloser interface {
	Close() error
}

// Error-reporting convention for host modules
//
// HostModule code surfaces failures back to Lua via three distinct
// mechanisms, each appropriate for a different situation. The choice
// drives how the error appears to the plugin author.
//
//  1. state.ArgError(idx, msg) — the Lua argument at position idx is
//     the wrong type, shape, or value. The Lua runtime frames the
//     message as "bad argument #N to 'fn' (msg)". Use during argument
//     parsing and after CheckString/CheckTable type assertions.
//
//  2. state.RaiseError("hive.<module>.<fn>: %s", err) — an operation
//     failed at runtime after argument parsing succeeded (e.g. JSON
//     decode hit invalid input, a kv store write returned an error, a
//     synchronous shell command exited non-zero). The "hive.<module>.<fn>"
//     prefix identifies the source so plugin authors can locate the
//     failure without traversing a stack trace.
//
//  3. Two-return (nil, errstr) — the call has already returned past the
//     synchronous Lua boundary (the error surfaces in a callback) and
//     RaiseError is therefore impossible. The async form of
//     hive.sh.output is the only current case: it cannot raise on
//     non-zero exit because the callback fires after the original call
//     has returned.
//
// Module-specific errors raised by case (2) should always include the
// "hive.<module>.<fn>" prefix; case (1) messages are framed by the
// runtime and do not need a prefix.
//
// Logging convention for host modules
//
// Modules that emit logs take a Logger field set by Plugin.Init via
// p.logger.With().Str("module", "<name>").Logger(). Tests use
// zerolog.Nop(). The level guidance is:
//
//   - Debug — normal lifecycle events (operation started, finished).
//     High-volume; off by default in production.
//   - Warn  — unexpected failures the plugin author may not see, such
//     as a store error that the plugin pcall'd, or an error returned
//     from a Lua callback that the dispatcher swallowed.
//   - Error — reserved for failures the operator must investigate;
//     currently unused inside host modules.
//
// Errors that are raised back to Lua via case (2) above are *also*
// logged at Warn when they come from out-of-band failures (KV store
// write, executor error). Errors that originate from the plugin's own
// input (JSON decode of malformed text, command-table type errors) are
// visible to the plugin author and not logged separately.
