import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { collabRoutes, useDocumentCollab, type CollabTarget } from '../lib/collab'
import { FakeEventSource } from '../test-utils/eventSource'
import type { ThreadList } from '../types'

const fetchMock = vi.fn<typeof fetch>()
vi.stubGlobal('fetch', fetchMock)

const list: ThreadList = { document_sha256: 'v1', viewer: { is_owner: false, can_comment: true }, threads: [] }

function respond(): void {
  fetchMock.mockImplementation((input, init) => Promise.resolve(answer(urlOf(input), init)))
}

function urlOf(input: RequestInfo | URL): string {
  return typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
}

function bodyOf(init: RequestInit | undefined): Record<string, unknown> {
  return JSON.parse(typeof init?.body === 'string' ? init.body : '{}') as Record<string, unknown>
}

function answer(url: string, init: RequestInit | undefined): Response {
  if (init?.method === 'POST' && url === '/api/v1/presence') return new Response(null, { status: 204 })
  if (init?.method === 'POST' && url.includes('/threads') && !url.includes('/comments')) {
    if (bodyOf(init).document_sha256 !== 'v1')
      return new Response(JSON.stringify({ kind: 'conflict', message: 'changed' }), { status: 409 })
    return new Response(JSON.stringify({ id: 't1' }), { status: 201 })
  }
  return new Response(JSON.stringify(list), { status: 200 })
}

function calls(match: (url: string, method: string) => boolean): number {
  return fetchMock.mock.calls.filter(([url, init]) => match(urlOf(url), init?.method ?? 'GET')).length
}

const threadReads = () => calls((url, method) => method === 'GET' && url.endsWith('/threads'))
const heartbeats = () =>
  fetchMock.mock.calls.filter(([url]) => urlOf(url) === '/api/v1/presence').map(([, init]) => bodyOf(init))

function start(target: CollabTarget = { kind: 'link', slug: 'abc' }, sha = 'v1') {
  const reloadDocument = vi.fn().mockResolvedValue(undefined)
  const onClosed = vi.fn()
  const scope = effectScope()
  const collab = scope.run(() => useDocumentCollab(target, { renderedSha: ref(sha), reloadDocument, onClosed }))!
  return { collab, reloadDocument, onClosed, scope, stream: FakeEventSource.latest() }
}

describe('collabRoutes', () => {
  it('names the owner and link routes', () => {
    const owner = collabRoutes({ kind: 'owner', apiPrefix: '/api/v1/shared', path: 'a b/c.md' })
    expect(owner.threads).toBe('/api/v1/shared/threads/a%20b/c.md')
    expect(owner.events).toBe('/api/v1/shared/events/a%20b/c.md')
    expect(owner.thread('t1')).toBe('/api/v1/threads/t1')

    const link = collabRoutes({ kind: 'link', slug: 'abc' })
    expect(link.threads).toBe('/api/v1/public/abc/threads')
    expect(link.create).toBe('/api/v1/links/abc/threads')
    expect(link.comment('c1')).toBe('/api/v1/links/abc/comments/c1')
  })
})

describe('useDocumentCollab', () => {
  beforeEach(() => {
    fetchMock.mockReset()
    respond()
  })
  afterEach(() => vi.useRealTimers())

  it('re-reads threads and the body on every hello, and threads on a threads event', async () => {
    const { collab, reloadDocument, stream, scope } = start()
    expect(stream.url).toBe('/api/v1/public/abc/events')

    stream.emit('hello', { id: 'p_1', token: 'secret' })
    await flushPromises()
    expect(threadReads()).toBe(1)
    expect(reloadDocument).toHaveBeenCalledTimes(1)
    expect(collab.connected.value).toBe(true)
    expect(collab.viewer.value?.can_comment).toBe(true)

    stream.emit('threads')
    await flushPromises()
    expect(threadReads()).toBe(2)

    stream.emit('document', { sha256: 'v2' })
    await flushPromises()
    expect(reloadDocument).toHaveBeenCalledTimes(2)
    expect(threadReads()).toBe(3)

    stream.fail()
    expect(collab.connected.value).toBe(false)
    stream.emit('hello', { id: 'p_1', token: 'again' })
    await flushPromises()
    expect(threadReads()).toBe(4)
    scope.stop()
  })

  it('sends the stream token, the anonymous name, and the selection in heartbeats', async () => {
    const { collab, stream, scope } = start()
    stream.emit('hello', { id: 'p_1', token: 'secret' })
    await flushPromises()
    const first = heartbeats()[0]
    expect(first.token).toBe('secret')
    expect(first.name).toMatch(/^[A-Z][a-z]+ [A-Z][a-z]+$/)

    collab.setSelection({ start_line: 3, end_line: 4 })
    await flushPromises()
    const last = heartbeats().at(-1)!
    expect(last.selection).toEqual({ start_line: 3, end_line: 4 })
    scope.stop()
  })

  it('throttles selection heartbeats', async () => {
    vi.useFakeTimers()
    const { collab, stream, scope } = start()
    stream.emit('hello', { id: 'p_1', token: 'secret' })
    await vi.advanceTimersByTimeAsync(0)
    const before = heartbeats().length

    for (let line = 1; line <= 10; line++) collab.setSelection({ start_line: line, end_line: line })
    await vi.advanceTimersByTimeAsync(100)
    expect(heartbeats().length - before).toBeLessThanOrEqual(2)
    await vi.advanceTimersByTimeAsync(1000)
    const last = heartbeats().at(-1)!
    expect(last.selection).toEqual({ start_line: 10, end_line: 10 })
    scope.stop()
  })

  it('stops and reports a closed stream', () => {
    const { onClosed, stream, scope } = start()
    stream.emit('closed')
    expect(onClosed).toHaveBeenCalledOnce()
    expect(stream.readyState).toBe(FakeEventSource.CLOSED)
    scope.stop()
  })

  it('reconnects a stream the browser gave up on', async () => {
    vi.useFakeTimers()
    const { stream, scope } = start()
    stream.fail(true)
    await vi.advanceTimersByTimeAsync(6000)
    expect(FakeEventSource.instances).toHaveLength(2)
    scope.stop()
  })

  it('posts a thread against the rendered version and explains a conflict', async () => {
    const ok = start()
    expect(await ok.collab.createThread(null, 'hi')).toBeNull()
    const post = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST' && bodyOf(init).body === 'hi')!
    expect(urlOf(post[0])).toBe('/api/v1/links/abc/threads')
    ok.scope.stop()

    const stale = start({ kind: 'link', slug: 'abc' }, 'v0')
    expect(await stale.collab.createThread(null, 'hi')).toContain('document changed')
    expect(stale.reloadDocument).toHaveBeenCalled()
    stale.scope.stop()
  })
})
