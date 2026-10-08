import { describe, expect, it } from 'vitest'
import { useTerminalOutcomes } from '../useTerminalOutcomes'

describe('terminal outcomes', () => {
  it('scopes multiline reports to a session and launch attempt', () => {
    const store = useTerminalOutcomes()
    const first = store.beginAttempt('one')
    const second = store.beginAttempt('one')
    const detail = 'Launch failed\n\n  indented detail'
    store.report('one', { reason: 'start-failed', detail }, second)
    store.report('one', { reason: 'completed', detail: 'stale success' }, first)
    expect(store.outcome('one')).toEqual({ reason: 'start-failed', detail })
    expect(store.outcome('two')).toBeNull()
    store.running('one')
    expect(store.outcome('one')).toBeNull()
    expect(store.wasRunning('one')).toBe(true)
  })

  it('forgets only an explicitly closed session', () => {
    const store = useTerminalOutcomes()
    for (const slug of ['one', 'two']) store.report(slug, { reason: 'terminated', detail: 'terminated' })
    store.forget('one')
    expect(store.outcome('one')).toBeNull()
    expect(store.outcome('two')?.reason).toBe('terminated')
  })

  it('does not reuse attempt identities after eviction or explicit close', () => {
    const store = useTerminalOutcomes()
    const attempt = store.beginAttempt('old')
    for (let i = 0; i < 64; i++) store.beginAttempt(`session-${i}`)
    expect(store.outcome('old')).toBeNull()
    const current = store.beginAttempt('old')
    store.report('old', { reason: 'start-failed', detail: 'stale' }, attempt)
    expect(store.outcome('old')).toBeNull()
    expect(current).toBeGreaterThan(attempt)
    store.forget('old')
    expect(store.beginAttempt('old')).toBeGreaterThan(current)
  })
})
