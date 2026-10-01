import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  MAX_INPUT_FRAME_BYTES,
  createTerminalClient,
  decodeFrame,
  encodeInputFrames,
  encodePasteFrames,
} from '../terminalClient'

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice', () => ({
  Endpoint: vi.fn(),
}))

const encoder = new TextEncoder()
const endpoint = {
  httpBaseURL: 'http://127.0.0.1:58006',
  wsURL: 'ws://127.0.0.1:58006/api/terminal/stream',
  token: 'tok-123',
}

// Mirrors encodeOutputFrame in cmd/desktop/internal/adapter/httpapi/terminal_stream.go.
function outputFrame(windowId: string, paneId: string, data: string): ArrayBuffer {
  const win = encoder.encode(windowId)
  const pane = encoder.encode(paneId)
  const body = encoder.encode(data)
  const bytes = new Uint8Array(3 + win.length + pane.length + body.length)
  bytes[0] = 0x00
  bytes[1] = win.length
  bytes.set(win, 2)
  bytes[2 + win.length] = pane.length
  bytes.set(pane, 3 + win.length)
  bytes.set(body, 3 + win.length + pane.length)
  return bytes.buffer
}

function jsonFrame(kind: number, payload: unknown): ArrayBuffer {
  const body = encoder.encode(JSON.stringify(payload))
  const bytes = new Uint8Array(1 + body.length)
  bytes[0] = kind
  bytes.set(body, 1)
  return bytes.buffer
}

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('terminal frame codec', () => {
  it('decodes an output frame into its ids and raw bytes', () => {
    const frame = decodeFrame(outputFrame('@12', '%34', 'hello'))
    expect(frame).toMatchObject({ type: 'output', windowId: '@12', paneId: '%34' })
    expect(new TextDecoder().decode((frame as { data: Uint8Array }).data)).toBe('hello')
  })

  it('decodes an output frame with an empty payload', () => {
    const frame = decodeFrame(outputFrame('@1', '%1', ''))
    expect((frame as { data: Uint8Array }).data).toHaveLength(0)
  })

  it('decodes window events with their stable string kinds', () => {
    const layout = { split: 'leftright', x: 0, y: 0, width: 213, height: 55, cells: [
      { paneId: '%3', x: 0, y: 0, width: 106, height: 55 },
      { paneId: '%4', x: 107, y: 0, width: 106, height: 55 },
    ] }
    expect(decodeFrame(jsonFrame(0x01, { kind: 'renamed', windowId: '@2', name: 'shell', active: false, activePane: '%4', width: 213, height: 55, zoomed: false, layout })))
      .toEqual({ type: 'window', kind: 'renamed', state: { windowId: '@2', name: 'shell', active: false, activePane: '%4', width: 213, height: 55, zoomed: false, layout } })
  })

  it('decodes a layout change as tmux reporting the grid the window must render at', () => {
    expect(decodeFrame(jsonFrame(0x01, { kind: 'layout-changed', windowId: '@2', name: 'shell', active: true, activePane: '%2', width: 80, height: 24, zoomed: true, layout: { paneId: '%2', x: 0, y: 0, width: 80, height: 24 } })))
      .toEqual({ type: 'window', kind: 'layout-changed', state: { windowId: '@2', name: 'shell', active: true, activePane: '%2', width: 80, height: 24, zoomed: true, layout: { paneId: '%2', x: 0, y: 0, width: 80, height: 24 } } })
  })

  it('reads an unreported size and layout as 0 and null rather than inventing them', () => {
    expect(decodeFrame(jsonFrame(0x01, { kind: 'added', windowId: '@3' })))
      .toEqual({ type: 'window', kind: 'added', state: { windowId: '@3', name: '', active: false, activePane: '', width: 0, height: 0, zoomed: false, layout: null } })
  })

  it('decodes lifecycle events', () => {
    expect(decodeFrame(jsonFrame(0x02, { kind: 'exited', windowId: '', message: 'overflow' })))
      .toEqual({ type: 'lifecycle', kind: 'exited', windowId: '', message: 'overflow' })
  })

  it('rejects empty, truncated and unknown frames', () => {
    expect(decodeFrame(new Uint8Array(0))).toBeNull()
    expect(decodeFrame(new Uint8Array([0x00, 0x04, 0x40]))).toBeNull() // declares 4 id bytes, carries 1
    expect(decodeFrame(new Uint8Array([0x7f, 0x01]))).toBeNull()
    expect(decodeFrame(new Uint8Array([0x01, 0x7b]))).toBeNull() // '{' is not valid JSON
  })

  it('encodes an input frame as [0x10][len][id][bytes]', () => {
    const [frame] = encodeInputFrames('@12', 'ab')
    expect(Array.from(frame)).toEqual([0x10, 3, 0x40, 0x31, 0x32, 0x61, 0x62])
  })

  it('emits no frame for empty input', () => {
    expect(encodeInputFrames('@1', '')).toEqual([])
  })

  it('chunks input past the 4 KiB frame cap without losing bytes', () => {
    const payload = 'a'.repeat(5000)
    const frames = encodeInputFrames('@12', payload)

    expect(frames.length).toBe(2)
    for (const frame of frames) {
      expect(frame.length).toBeLessThanOrEqual(MAX_INPUT_FRAME_BYTES)
      expect(frame[0]).toBe(0x10)
      expect(frame[1]).toBe(3)
    }
    const rejoined = frames.map((frame) => new TextDecoder().decode(frame.subarray(5))).join('')
    expect(rejoined).toBe(payload)
  })

  it('frames a paste as chunks plus a commit', () => {
    const frames = encodePasteFrames('@12', 'ab')

    expect(frames.length).toBe(2)
    expect(Array.from(frames[0])).toEqual([0x11, 3, 0x40, 0x31, 0x32, 0x61, 0x62])
    expect(Array.from(frames[1])).toEqual([0x12, 3, 0x40, 0x31, 0x32])
  })

  // tmux writes one \r per \n in the buffer, so a CRLF would submit twice.
  it('normalizes line endings to \\n', () => {
    const [chunk] = encodePasteFrames('@1', 'one\r\ntwo\rthree\nfour')
    expect(new TextDecoder().decode(chunk.subarray(4))).toBe('one\ntwo\nthree\nfour')
  })

  it('emits no frame for an empty paste', () => {
    expect(encodePasteFrames('@1', '')).toEqual([])
  })

  it('chunks a paste past the frame cap and commits once', () => {
    const frames = encodePasteFrames('@12', 'a'.repeat(5000))

    expect(frames.length).toBe(3)
    expect(frames.filter((frame) => frame[0] === 0x12).length).toBe(1)
    expect(frames[frames.length - 1][0]).toBe(0x12)
    for (const frame of frames) expect(frame.length).toBeLessThanOrEqual(MAX_INPUT_FRAME_BYTES)
  })
})

describe('createTerminalClient', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn()
    globalThis.fetch = fetchMock as unknown as typeof fetch
  })

  it('attaches with the bearer token and returns the window set', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {
      windows: [{ windowId: '@1', name: 'agent', active: true, width: 213, height: 55 }],
    }))

    const result = await createTerminalClient(endpoint).attach('hive-abc', 120, 40)

    // The cols/rows posted are a vote; the sizes that come back are tmux's.
    expect(result.windows).toEqual([{ windowId: '@1', name: 'agent', active: true, activePane: '', width: 213, height: 55, zoomed: false, layout: null }])
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/attach')
    expect(init.headers.Authorization).toBe('Bearer tok-123')
    expect(JSON.parse(init.body)).toEqual({ slug: 'hive-abc', cols: 120, rows: 40 })
  })

  it('lists every session’s windows without attaching, in one call', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {
      sessions: { 'hive-abc': [{ windowId: '@2', name: 'shell', active: false, width: 120, height: 40 }] },
    }))

    const result = await createTerminalClient(endpoint).listWindows(['hive-abc', 'hive-never-spawned'])

    expect(result).toEqual({ 'hive-abc': [{ windowId: '@2', name: 'shell', active: false, activePane: '', width: 120, height: 40, zoomed: false, layout: null }] })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/windows/list')
    expect(JSON.parse(init.body)).toEqual({ slugs: ['hive-abc', 'hive-never-spawned'] })
  })

  it('treats a 204 as success for the void operations', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
    const client = createTerminalClient(endpoint)

    await expect(client.resize('hive-abc', 100, 30)).resolves.toBeUndefined()
    await expect(client.selectWindow('hive-abc', '@2')).resolves.toBeUndefined()
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ slug: 'hive-abc', windowId: '@2' })
  })

  it('launches an agent by profile on the authenticated control plane', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { windowId: '@8' }))
    expect(await createTerminalClient(endpoint).newAgentWindow('hive-abc', 'codex')).toEqual({ windowId: '@8' })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/windows/agent')
    expect(init.headers.Authorization).toBe('Bearer tok-123')
    expect(JSON.parse(init.body)).toEqual({ slug: 'hive-abc', agent: 'codex' })
  })

  it('moves a window to a position and answers with the order tmux settled on', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {
      windows: [
        { windowId: '@2', name: 'shell', active: false, width: 120, height: 40 },
        { windowId: '@1', name: 'agent', active: true, width: 120, height: 40 },
      ],
    }))

    const result = await createTerminalClient(endpoint).moveWindow('hive-abc', '@1', 1)

    // The reply is a window set, not an acknowledgement: tmux owns the order.
    expect(result.windows.map((window) => window.windowId)).toEqual(['@2', '@1'])
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/windows/move')
    expect(JSON.parse(init.body)).toEqual({ slug: 'hive-abc', windowId: '@1', position: 1 })
  })

  it('starts a session and reports whether the call is what spawned it', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { started: true }))

    const result = await createTerminalClient(endpoint).start('hive-abc')

    expect(result).toEqual({ started: true })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/start')
    expect(JSON.parse(init.body)).toEqual({ slug: 'hive-abc' })
  })

  it('kills a session and reports whether there was one to kill', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { killed: false }))

    const result = await createTerminalClient(endpoint).kill('hive-abc')

    expect(result).toEqual({ killed: false })
    expect(fetchMock.mock.calls[0][0]).toBe('http://127.0.0.1:58006/api/terminal/kill')
  })

  it('reports what a window is running before it is closed', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { running: true, command: 'claude' }))

    const result = await createTerminalClient(endpoint).windowForeground('hive-abc', '@1')

    expect(result).toEqual({ running: true, command: 'claude' })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/windows/foreground')
    expect(JSON.parse(init.body)).toEqual({ slug: 'hive-abc', windowId: '@1' })
  })

  // An answer carrying no verdict is an unknown, and the caller kills the window
  // on the strength of this one — so an unknown is something running.
  it('reads a window with no verdict as running rather than idle', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {}))

    await expect(createTerminalClient(endpoint).windowForeground('hive-abc', '@1'))
      .resolves.toEqual({ running: true, command: '' })
  })

  it('splits a pane and answers the id of the pane it made', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { paneId: '%7' }))

    const result = await createTerminalClient(endpoint).splitPane('hive-abc', '%1', 'vertical')

    expect(result).toEqual({ paneId: '%7' })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/panes/split')
    expect(JSON.parse(init.body)).toEqual({ slug: 'hive-abc', paneId: '%1', direction: 'vertical' })
  })

  it('reads a split that named no pane as an empty id', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {}))

    await expect(createTerminalClient(endpoint).splitPane('hive-abc', '%1', 'horizontal'))
      .resolves.toEqual({ paneId: '' })
  })

  // The server requires explicit fields: an empty direction means the target
  // pane, and a zero resize axis means unchanged.
  it('spells an absent direction and axis out as empty and zero', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
    const client = createTerminalClient(endpoint)

    await client.selectPane('hive-abc', '%1')
    await client.selectPane('hive-abc', '%1', 'left')
    await client.resizePane('hive-abc', '%1', { width: 50 })

    expect(fetchMock.mock.calls[0][0]).toBe('http://127.0.0.1:58006/api/terminal/panes/select')
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ slug: 'hive-abc', paneId: '%1', direction: '' })
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ slug: 'hive-abc', paneId: '%1', direction: 'left' })
    expect(fetchMock.mock.calls[2][0]).toBe('http://127.0.0.1:58006/api/terminal/panes/resize')
    expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ slug: 'hive-abc', paneId: '%1', width: 50, height: 0 })
  })

  it('closes and zooms a pane by its id alone', async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }))
    const client = createTerminalClient(endpoint)

    await expect(client.closePane('hive-abc', '%1')).resolves.toBeUndefined()
    await expect(client.zoomPane('hive-abc', '%1')).resolves.toBeUndefined()

    expect(fetchMock.mock.calls[0][0]).toBe('http://127.0.0.1:58006/api/terminal/panes/close')
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ slug: 'hive-abc', paneId: '%1' })
    expect(fetchMock.mock.calls[1][0]).toBe('http://127.0.0.1:58006/api/terminal/panes/zoom')
    expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ slug: 'hive-abc', paneId: '%1' })
  })

  it('reports what a pane is running before it is closed', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, { running: false, command: '' }))

    const result = await createTerminalClient(endpoint).paneForeground('hive-abc', '%1')

    expect(result).toEqual({ running: false, command: '' })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('http://127.0.0.1:58006/api/terminal/panes/foreground')
    expect(JSON.parse(init.body)).toEqual({ slug: 'hive-abc', paneId: '%1' })
  })

  // A missing verdict fails closed because callers use it to decide whether
  // killing the pane is safe.
  it('reads a pane with no verdict as running rather than idle', async () => {
    fetchMock.mockResolvedValue(jsonResponse(200, {}))

    await expect(createTerminalClient(endpoint).paneForeground('hive-abc', '%1'))
      .resolves.toEqual({ running: true, command: '' })
  })

  it('surfaces the core error message and its kind from a failed control action', async () => {
    fetchMock.mockResolvedValue(jsonResponse(404, { kind: 'not_found', message: 'no terminal is attached for that slug' }))

    // The kind is what tells a session that is not running apart from tmux
    // being unusable; the view branches on it rather than on the message.
    await expect(createTerminalClient(endpoint).detach('gone'))
      .rejects.toMatchObject({ message: 'no terminal is attached for that slug', kind: 'not_found' })
  })

  it('opens the stream on the versioned URL with the token in the query', () => {
    const created: string[] = []
    class FakeSocket {
      binaryType = 'blob'
      constructor(url: string) { created.push(url) }
    }
    globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket

    const socket = createTerminalClient(endpoint).openStream('hive-abc')

    expect(socket.binaryType).toBe('arraybuffer')
    expect(created[0]).toBe('ws://127.0.0.1:58006/api/terminal/stream?slug=hive-abc&token=tok-123&v=2')
  })
})
