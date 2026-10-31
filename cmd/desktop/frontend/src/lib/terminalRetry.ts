// How long a server's terminal waits before each attach after an ssh drop. The
// cap gives up after about a minute; the Reconnect button still works after.
export const RETRY_DELAYS_MS = [1000, 2000, 5000]
export const MAX_RETRIES = 12

export type Retry =
  | { kind: 'idle' }
  | { kind: 'waiting'; attempt: number; delayMs: number }
  | { kind: 'attaching'; attempt: number }
  | { kind: 'stabilizing'; attempt: number }
  | { kind: 'gave-up' }

export type RetryEvent =
  /** The stream died or an attach failed in a way another attach might fix. */
  | { type: 'failed' }
  /** An attach began, from the retry timer or the Reconnect button. */
  | { type: 'attaching' }
  | { type: 'opened' }
  /** The stream stayed up long enough to earn the retries back. */
  | { type: 'stable' }
  /** The session has a clear answer, such as gone or stopped, so retrying cannot help. */
  | { type: 'ended' }

export const NO_RETRY: Retry = { kind: 'idle' }

function attemptOf(state: Retry): number {
  switch (state.kind) {
    case 'idle':
      return 0
    case 'gave-up':
      return MAX_RETRIES
    default:
      return state.attempt
  }
}

/**
 * The retry decision for a server's terminal. An event that does not apply to
 * the state leaves it alone, so a second report of one drop arms nothing new.
 */
export function nextRetry(state: Retry, event: RetryEvent): Retry {
  switch (event.type) {
    case 'failed': {
      if (state.kind === 'waiting' || state.kind === 'gave-up') return state
      const attempt = attemptOf(state)
      if (attempt >= MAX_RETRIES) return { kind: 'gave-up' }
      return {
        kind: 'waiting',
        attempt: attempt + 1,
        delayMs: RETRY_DELAYS_MS[Math.min(attempt, RETRY_DELAYS_MS.length - 1)],
      }
    }
    case 'attaching':
      return { kind: 'attaching', attempt: attemptOf(state) }
    case 'opened':
      // The socket opens before the first frame, so an open earns nothing back.
      return state.kind === 'attaching' ? { kind: 'stabilizing', attempt: state.attempt } : state
    case 'stable':
      return state.kind === 'stabilizing' ? NO_RETRY : state
    case 'ended':
      return NO_RETRY
  }
}
