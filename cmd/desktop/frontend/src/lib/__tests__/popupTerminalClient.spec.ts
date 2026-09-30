import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  createPopupTerminalClient,
  decodeFrame,
  encodeInputFrames,
  MAX_INPUT_FRAME_BYTES,
  PopupTerminalRequestError,
} from '../popupTerminalClient'

const encoder = new TextEncoder()

const endpoint = {
  httpBaseURL: 'http://127.0.0.1:58006',
  wsURL: 'ws://127.0.0.1:58006/api/terminal/pty/stream',
  token: 'tok-123',
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('createPopupTerminalClient', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    globalThis.fetch = fetchMock as unknown as typeof fetch
  })

  it('posts a launch and fills in what the server left out', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { id: 't1', title: 'zsh', dir: '/tmp/checkout' }))

    const term = await createPopupTerminalClient(endpoint).open({ sessionSlug: 'hive-abc' })

    expect(term).toEqual({ id: 't1', title: 'zsh', dir: '/tmp/checkout', command: '', cols: 0, rows: 0 })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/popup/open')
    expect(init.headers.Authorization).toBe('Bearer tok-123')
    expect(JSON.parse(init.body)).toEqual({ sessionSlug: 'hive-abc' })
  })

  it('carries the core classification on a failure', async () => {
    fetchMock.mockResolvedValue(jsonResponse(404, { message: 'no such terminal', kind: 'not_found' }))

    await expect(createPopupTerminalClient(endpoint).close('t9')).rejects.toMatchObject({
      constructor: PopupTerminalRequestError,
      kind: 'not_found',
      message: 'no such terminal',
    })
  })

  it('opens the stream for one terminal id', () => {
    const created: string[] = []
    class FakeSocket {
      binaryType = 'blob'
      constructor(url: string) { created.push(url) }
    }
    globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket

    createPopupTerminalClient(endpoint).openStream('t1')

    expect(created[0]).toBe('ws://127.0.0.1:58006/api/terminal/pty/stream?id=t1&token=tok-123&v=1')
  })
})

// One socket carries one terminal, so an output frame is a tag and the bytes.
describe('popup terminal frames', () => {
  it('splits input at the server cap', () => {
    const frames = encodeInputFrames('x'.repeat(MAX_INPUT_FRAME_BYTES * 2))

    expect(frames).toHaveLength(3)
    for (const frame of frames) {
      expect(frame[0]).toBe(0x10)
      expect(frame.length).toBeLessThanOrEqual(MAX_INPUT_FRAME_BYTES)
    }
  })

  it('decodes output and exit', () => {
    const output = new Uint8Array([0x00, ...encoder.encode('hello')])
    expect(decodeFrame(output)).toEqual({ type: 'output', data: encoder.encode('hello') })

    const exit = new Uint8Array([0x01, ...encoder.encode(JSON.stringify({ reason: 'exited with status 0' }))])
    expect(decodeFrame(exit)).toEqual({ type: 'exit', reason: 'exited with status 0' })
  })

  it('reads an unknown or empty frame as nothing rather than throwing', () => {
    expect(decodeFrame(new Uint8Array([]))).toBeNull()
    expect(decodeFrame(new Uint8Array([0x7f, 1, 2]))).toBeNull()
  })
})
