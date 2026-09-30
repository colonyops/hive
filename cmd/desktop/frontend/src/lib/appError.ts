// Every error a Go binding call returns arrives here classified. The Go core
// assigns a Kind (internal/app/errors.go), the Wails adapter serializes it
// through MarshalError, and the runtime hands it back as the thrown
// exception's `cause`.
//
// Match on the Kind, never on the message. A message is written for a
// developer reading a log and changes freely; a Kind is a contract.

/** The closed set of failure classifications the Go core assigns. */
export type AppErrorKind =
  | 'internal'
  | 'invalid'
  | 'not_found'
  | 'conflict'
  | 'unauthenticated'
  | 'unavailable'

export interface AppError {
  kind: AppErrorKind
  message: string
}

const KINDS: readonly AppErrorKind[] = [
  'internal',
  'invalid',
  'not_found',
  'conflict',
  'unauthenticated',
  'unavailable',
]

function isKind(value: unknown): value is AppErrorKind {
  return typeof value === 'string' && (KINDS as readonly string[]).includes(value)
}

/**
 * Narrows a thrown binding error to the Kind the Go core assigned, or null
 * when the error did not come from one — a network fault, a bug in the
 * frontend, an older backend. A null result means "we cannot say", so callers
 * should treat it as unhandled rather than as any particular kind.
 */
export function appErrorKind(error: unknown): AppErrorKind | null {
  if (typeof error !== 'object' || error === null) return null
  const cause = (error as { cause?: unknown }).cause
  if (typeof cause !== 'object' || cause === null) return null
  const kind = (cause as { kind?: unknown }).kind
  return isKind(kind) ? kind : null
}

/** Reads the core's user-facing message, or "" when there is none. */
export function appErrorMessage(error: unknown): string {
  if (typeof error !== 'object' || error === null) return ''
  const cause = (error as { cause?: unknown }).cause
  if (typeof cause !== 'object' || cause === null) return ''
  const message = (cause as { message?: unknown }).message
  return typeof message === 'string' ? message : ''
}

/**
 * The fullest text available for a caught error: the core's message when the
 * call reached Go, the thrown message when it did not, and `fallback` when
 * neither says anything. For a surface that shows the user what went wrong —
 * still never for deciding what to do about it, which is the Kind's job.
 */
export function errorText(error: unknown, fallback: string): string {
  return appErrorMessage(error) || (error instanceof Error && error.message ? error.message : fallback)
}
