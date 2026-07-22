import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetToastsForTests, useToasts } from '../useToasts'

describe('useToasts', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    resetToastsForTests()
  })

  afterEach(() => {
    resetToastsForTests()
    vi.useRealTimers()
  })

  it('appends to the shared queue', () => {
    const { toasts, showToast } = useToasts()
    const id = showToast('Saved', { severity: 'success', body: 'Profile updated' })
    expect(toasts.value).toEqual([expect.objectContaining({ id, message: 'Saved', severity: 'success', body: 'Profile updated', duration: 4000 })])
  })

  it('auto-dismisses toasts after their duration', () => {
    const { toasts, showToast } = useToasts()
    showToast('Soon gone', { duration: 10 })
    vi.advanceTimersByTime(10)
    expect(toasts.value).toEqual([])
  })

  it('auto-dismisses error toasts so durable Activity remains the long-term record', () => {
    const { toasts, showToast } = useToasts()
    showToast('Failed', { severity: 'error', duration: 10 })
    vi.advanceTimersByTime(10)
    expect(toasts.value).toEqual([])
  })

  it('clears the queue and cancels pending timers', () => {
    const { toasts, showToast, clearToasts } = useToasts()
    showToast('One', { duration: 10 })
    clearToasts()
    vi.advanceTimersByTime(10)
    expect(toasts.value).toEqual([])
  })
})
