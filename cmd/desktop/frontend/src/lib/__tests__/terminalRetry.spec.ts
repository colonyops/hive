import { describe, expect, it } from 'vitest'
import { MAX_RETRIES, NO_RETRY, nextRetry, type Retry, type RetryEvent } from '../terminalRetry'

const waiting = (attempt: number, delayMs: number): Retry => ({ kind: 'waiting', attempt, delayMs })
const attaching = (attempt: number): Retry => ({ kind: 'attaching', attempt })
const stabilizing = (attempt: number): Retry => ({ kind: 'stabilizing', attempt })
const gaveUp: Retry = { kind: 'gave-up' }

function run(state: Retry, ...events: RetryEvent['type'][]): Retry {
  return events.reduce((current, type) => nextRetry(current, { type }), state)
}

describe('nextRetry', () => {
  it.each<[string, Retry, RetryEvent['type'], Retry]>([
    ['a first failure waits 1s', NO_RETRY, 'failed', waiting(1, 1000)],
    ['a failed attach waits longer', attaching(1), 'failed', waiting(2, 2000)],
    ['the delay stops growing at 5s', attaching(2), 'failed', waiting(3, 5000)],
    ['a later failure stays at 5s', attaching(11), 'failed', waiting(12, 5000)],
    ['a drop before the stream is stable counts', stabilizing(3), 'failed', waiting(4, 5000)],
    ['the last failed attach gives up', attaching(MAX_RETRIES), 'failed', gaveUp],
    ['a drop at the cap gives up', stabilizing(MAX_RETRIES), 'failed', gaveUp],
    ['a second report of a drop changes nothing', waiting(2, 2000), 'failed', waiting(2, 2000)],
    ['a failure after giving up changes nothing', gaveUp, 'failed', gaveUp],

    ['the timer starts an attach', waiting(2, 2000), 'attaching', attaching(2)],
    ['Reconnect from a live stream starts at zero', NO_RETRY, 'attaching', attaching(0)],
    ['Reconnect keeps a stream on probation', stabilizing(4), 'attaching', attaching(4)],
    ['Reconnect after giving up stays at the cap', gaveUp, 'attaching', attaching(MAX_RETRIES)],

    ['an attach that opens is on probation', attaching(3), 'opened', stabilizing(3)],
    ['the first open has nothing to earn back', NO_RETRY, 'opened', NO_RETRY],
    ['an open while waiting changes nothing', waiting(2, 2000), 'opened', waiting(2, 2000)],

    ['a stable stream earns the retries back', stabilizing(5), 'stable', NO_RETRY],
    ['a stable report while waiting changes nothing', waiting(2, 2000), 'stable', waiting(2, 2000)],

    ['a clear answer stops the wait', waiting(2, 2000), 'ended', NO_RETRY],
    ['a clear answer clears the count', attaching(7), 'ended', NO_RETRY],
    ['a clear answer leaves giving up behind', gaveUp, 'ended', NO_RETRY],
  ])('%s', (_name, state, event, want) => {
    expect(nextRetry(state, { type: event })).toEqual(want)
  })

  it('gives up after 12 failed attaches', () => {
    let state = NO_RETRY
    let attaches = 0
    for (let failure = 0; failure < 40 && state.kind !== 'gave-up'; failure++) {
      state = nextRetry(state, { type: 'failed' })
      if (state.kind !== 'waiting') break
      state = nextRetry(state, { type: 'attaching' })
      attaches++
    }
    expect(state).toEqual(gaveUp)
    expect(attaches).toBe(12)
  })

  it('gives up when each stream opens and drops at once', () => {
    let state: Retry = run(NO_RETRY, 'failed')
    for (let drop = 0; drop < 40 && state.kind === 'waiting'; drop++) {
      state = run(state, 'attaching', 'opened', 'failed')
    }
    expect(state).toEqual(gaveUp)
  })

  it('gets a full set of retries again after a stable stream', () => {
    const state = run(attaching(11), 'opened', 'stable', 'failed')
    expect(state).toEqual(waiting(1, 1000))
  })

  it('gets one more attach from Reconnect after giving up, and gives up again', () => {
    expect(run(gaveUp, 'attaching', 'failed')).toEqual(gaveUp)
  })

  it('can earn the retries back from a Reconnect after giving up', () => {
    expect(run(gaveUp, 'attaching', 'opened', 'stable')).toEqual(NO_RETRY)
  })
})
