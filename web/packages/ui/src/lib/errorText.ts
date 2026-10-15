function causeMessage(error: unknown): string {
  if (typeof error !== 'object' || error === null) return ''
  const cause = (error as { cause?: unknown }).cause
  if (typeof cause !== 'object' || cause === null) return ''
  const message = (cause as { message?: unknown }).message
  return typeof message === 'string' ? message : ''
}

/**
 * The fullest text available for a caught error: the core's message when the
 * call reached Go, the thrown message (or thrown string) when it did not, and
 * `fallback` when neither says anything. For a surface that shows the user
 * what went wrong -- still never for deciding what to do about it, which is
 * the Kind's job.
 */
export function errorText(error: unknown, fallback: string): string {
  const message = causeMessage(error) || (error instanceof Error ? error.message : '')
  // The runtime can reject with a bare string rather than an Error.
  return message || (typeof error === 'string' && error) || fallback
}
