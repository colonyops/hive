import { flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { useResource } from '../useResource'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const options = { initial: [] as string[], errorFallback: 'Could not load rows.' }

describe('useResource', () => {
  it('starts with the initial value and nothing loaded', () => {
    const rows = useResource(() => Promise.resolve(['a']), options)

    expect(rows.data.value).toEqual([])
    expect(rows.loading.value).toBe(false)
    expect(rows.loaded.value).toBe(false)
    expect(rows.error.value).toBeNull()
  })

  it('loads data and reports loading while the request runs', async () => {
    const request = deferred<string[]>()
    const rows = useResource(() => request.promise, options)

    const reload = rows.reload()
    expect(rows.loading.value).toBe(true)

    request.resolve(['a', 'b'])
    await reload

    expect(rows.data.value).toEqual(['a', 'b'])
    expect(rows.loading.value).toBe(false)
    expect(rows.loaded.value).toBe(true)
    expect(rows.error.value).toBeNull()
  })

  it('keeps the last good data and records the error text on failure', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(['a']).mockRejectedValueOnce(new Error('boom'))
    const rows = useResource<string[]>(fetch, options)

    await rows.reload()
    await rows.reload()

    expect(rows.data.value).toEqual(['a'])
    expect(rows.error.value).toBe('boom')
    expect(rows.loaded.value).toBe(true)
    expect(rows.loading.value).toBe(false)
  })

  it('falls back to the configured text when the failure has no message', async () => {
    const rows = useResource<string[]>(() => Promise.reject(new Error('')), options)

    await rows.reload()

    expect(rows.error.value).toBe('Could not load rows.')
    expect(rows.loaded.value).toBe(true)
  })

  it('prefers the message the Go core attached to the error', async () => {
    const fromCore = new Error('wrapped', { cause: { kind: 'unavailable', message: 'hive is not running' } })
    const rows = useResource<string[]>(() => Promise.reject(fromCore), options)

    await rows.reload()

    expect(rows.error.value).toBe('hive is not running')
  })

  it('clears the error once a later load succeeds', async () => {
    const fetch = vi.fn().mockRejectedValueOnce(new Error('boom')).mockResolvedValueOnce(['a'])
    const rows = useResource<string[]>(fetch, options)

    await rows.reload()
    await rows.reload()

    expect(rows.error.value).toBeNull()
    expect(rows.data.value).toEqual(['a'])
  })

  it('runs one follow-up for every reload that arrives during a request', async () => {
    const first = deferred<string[]>()
    const second = deferred<string[]>()
    const fetch = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const rows = useResource<string[]>(fetch, options)

    const initial = rows.reload()
    const duringA = rows.reload()
    const duringB = rows.reload()
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(duringB).toBe(duringA)

    first.resolve(['stale'])
    await initial
    await flushPromises()
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(rows.data.value).toEqual(['stale'])
    expect(rows.loading.value).toBe(true)

    second.resolve(['fresh'])
    await duringA
    expect(rows.data.value).toEqual(['fresh'])
    expect(rows.loading.value).toBe(false)
  })

  it('starts a new request for a reload that arrives after the previous one settled', async () => {
    const fetch = vi.fn().mockResolvedValue(['a'])
    const rows = useResource<string[]>(fetch, options)

    await rows.reload()
    await rows.reload()

    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
