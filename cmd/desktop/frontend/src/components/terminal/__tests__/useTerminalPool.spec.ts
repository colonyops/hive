import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick, ref } from 'vue'
import type { TerminalClient } from '../../../lib/terminalClient'
import { HOLD_MS, useTerminalPool } from '../useTerminalPool'

const mocks = vi.hoisted(() => ({ useTerminalWindows: vi.fn() }))
vi.mock('../../../composables/useTerminalWindows', () => ({ useTerminalWindows: mocks.useTerminalWindows }))

function fakeSession(slug: string) {
  return {
    slug,
    tabs: ref([{ windowId: '@1' }, { windowId: '@2' }]),
    activeWindowId: ref('@1'),
    status: ref<'connecting' | 'live' | 'ended'>('live'),
    painted: ref(false),
    start: vi.fn().mockResolvedValue(undefined),
    select: vi.fn().mockResolvedValue(undefined),
    focusActive: vi.fn(),
    dispose: vi.fn(),
  }
}

const client = {} as TerminalClient

function setup(size = 2) {
  const sessions = new Map<string, ReturnType<typeof fakeSession>>()
  mocks.useTerminalWindows.mockImplementation((slug: string) => {
    const session = fakeSession(slug)
    sessions.set(slug, session)
    return session
  })
  const onEvicted = vi.fn()
  const scope = effectScope()
  const pool = scope.run(() => useTerminalPool({ size: ref(size), onEvicted }))!
  return { pool, sessions, onEvicted, scope }
}

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('useTerminalPool', () => {
  it('keeps the last attaches warm and evicts the least recently used', () => {
    const { pool, sessions, onEvicted } = setup(2)
    pool.open('a', client, '')
    pool.open('b', client, '')
    pool.open('a', client, '')
    pool.open('c', client, '')

    expect([...pool.pool.keys()].sort()).toEqual(['a', 'c'])
    expect(sessions.get('b')?.dispose).toHaveBeenCalled()
    expect(onEvicted).toHaveBeenCalledTimes(1)
    expect(mocks.useTerminalWindows).toHaveBeenCalledTimes(3)
  })

  it('holds the outgoing session on screen until the incoming one paints', async () => {
    const { pool, sessions } = setup()
    pool.open('a', client, '')
    await nextTick()
    expect(pool.visible.value).toBe(sessions.get('a'))

    pool.open('b', client, '')
    await nextTick()
    expect(pool.visible.value).toBe(sessions.get('a'))
    expect(pool.visibleSlug.value).toBe('a')

    sessions.get('b')!.painted.value = true
    await nextTick()
    expect(pool.visible.value).toBe(sessions.get('b'))
    expect(pool.visibleSlug.value).toBe('b')
  })

  it('reveals an unpainted session once the hold cap runs out', async () => {
    const { pool, sessions } = setup()
    pool.open('a', client, '')
    await nextTick()
    pool.open('b', client, '')
    await nextTick()
    expect(pool.visible.value).toBe(sessions.get('a'))
    vi.advanceTimersByTime(HOLD_MS)
    await nextTick()
    expect(pool.visible.value).toBe(sessions.get('b'))
  })

  it('selects the wanted window once a cold attach has started', async () => {
    const { pool, sessions } = setup()
    pool.open('a', client, '@2')
    await vi.runAllTimersAsync()
    expect(sessions.get('a')?.select).toHaveBeenCalledWith('@2')
  })

  it('re-attaches an ended session instead of reusing it', () => {
    const { pool, sessions } = setup()
    pool.open('a', client, '')
    const ended = sessions.get('a')!
    ended.status.value = 'ended'
    pool.open('a', client, '')
    expect(ended.dispose).toHaveBeenCalled()
    expect(pool.pool.get('a')).not.toBe(ended)
  })

  it('remembers the attached session across a detach, and forgets it when dropped', () => {
    const { pool } = setup()
    pool.open('a', client, '')
    pool.detach()
    expect(pool.activeSlug.value).toBe('')
    expect(pool.lastAttachedSlug.value).toBe('a')
    pool.drop('a')
    expect(pool.lastAttachedSlug.value).toBe('')
  })

  it('disposes every attach with its scope', () => {
    const { pool, sessions, scope } = setup()
    pool.open('a', client, '')
    pool.open('b', client, '')
    scope.stop()
    expect(sessions.get('a')?.dispose).toHaveBeenCalled()
    expect(sessions.get('b')?.dispose).toHaveBeenCalled()
    expect(pool.pool.size).toBe(0)
  })
})
