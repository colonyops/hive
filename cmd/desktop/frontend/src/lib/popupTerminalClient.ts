// The pop-up terminal's transport. Control actions are plain fetch calls
// against the loopback HTTP API; the data plane is a native WebSocket carrying
// the frames defined in internal/adapter/httpapi/pty_stream.go. One socket
// carries one terminal, so nothing on this wire is addressed:
//
//   server -> client
//     0x00 Output [0x00][raw bytes]
//     0x01 Exit   [0x01][JSON {reason}]
//   client -> server
//     0x10 Input  [0x10][raw bytes]

import { Endpoint } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popupterminalservice'
import type { PopupTerminalEndpoint } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type { PopupTerminalEndpoint }

/** Carried as ?v=; the server rejects anything else before the upgrade. */
export const POPUP_WIRE_VERSION = '1'

/** The server's whole-frame cap on one client -> server message. */
export const MAX_INPUT_FRAME_BYTES = 4 << 10

const FRAME_OUTPUT = 0x00
const FRAME_EXIT = 0x01
const FRAME_INPUT = 0x10

/** One open terminal. The id is the app process's own and dies with it. */
export interface PopupTerminalState {
  id: string
  title: string
  dir: string
  command: string
  cols: number
  rows: number
}

/**
 * What a launch asks for. An empty command opens an interactive shell.
 * `launcher` is a configured terminal-popup action id and brings its own
 * command — what it runs is the core's answer, never sent from here.
 */
export interface PopupTerminalRequest {
  launcher?: string
  sessionSlug?: string
  dir?: string
  command?: string
  cols?: number
  rows?: number
}

export type PopupTerminalFrame =
  | { type: 'output'; data: Uint8Array }
  | { type: 'exit'; reason: string }

/**
 * A control-plane failure carrying the core's own classification, so a caller
 * can tell a terminal that is gone (`not_found`) from PTYs being unusable
 * (`unavailable`) without reading the message.
 */
export class PopupTerminalRequestError extends Error {
  constructor(message: string, readonly kind: string) {
    super(message)
    this.name = 'PopupTerminalRequestError'
  }
}

export interface PopupTerminalClient {
  open(request: PopupTerminalRequest): Promise<PopupTerminalState>
  close(id: string): Promise<{ closed: boolean }>
  list(): Promise<PopupTerminalState[]>
  resize(id: string, cols: number, rows: number): Promise<void>
  openStream(id: string): WebSocket
}

const encoder = new TextEncoder()
const decoder = new TextDecoder()

/** Resolves the transport the webview was handed, or rejects with the reason. */
export async function getPopupTerminalEndpoint(): Promise<PopupTerminalEndpoint> {
  return await Endpoint()
}

export function createPopupTerminalClient(endpoint: PopupTerminalEndpoint): PopupTerminalClient {
  async function post<T>(path: string, body: unknown): Promise<T | null> {
    const response = await fetch(`${endpoint.httpBaseURL}/api/terminal/popup${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${endpoint.token}` },
      body: JSON.stringify(body),
    })
    if (!response.ok) throw await failure(response)
    if (response.status === 204) return null
    return (await response.json()) as T
  }

  return {
    async open(request) {
      const body = await post<Partial<PopupTerminalState>>('/open', request)
      return toTerminalState(body ?? {})
    },
    async close(id) {
      const body = await post<{ closed: boolean }>('/close', { id })
      return { closed: !!body?.closed }
    },
    async list() {
      const body = await post<{ terminals: Partial<PopupTerminalState>[] | null }>('/list', {})
      return (body?.terminals ?? []).map(toTerminalState)
    },
    async resize(id, cols, rows) { await post('/resize', { id, cols, rows }) },
    openStream(id) {
      const socket = new WebSocket(streamURL(endpoint, id))
      socket.binaryType = 'arraybuffer'
      return socket
    },
  }
}

/**
 * Builds the data-plane URL. The bearer token cannot ride a header on a browser
 * handshake, so it goes in the query string the server also accepts.
 */
function streamURL(endpoint: PopupTerminalEndpoint, id: string): string {
  const url = new URL(endpoint.wsURL)
  url.searchParams.set('id', id)
  url.searchParams.set('token', endpoint.token)
  url.searchParams.set('v', POPUP_WIRE_VERSION)
  return url.toString()
}

/**
 * Splits keystrokes into input frames no larger than the server's cap. Bytes
 * are reassembled in order on the far end, so a split inside a multi-byte
 * sequence is harmless.
 */
export function encodeInputFrames(data: string | Uint8Array): Uint8Array[] {
  const body = typeof data === 'string' ? encoder.encode(data) : data
  const budget = MAX_INPUT_FRAME_BYTES - 1

  const frames: Uint8Array[] = []
  for (let offset = 0; offset < body.length; offset += budget) {
    const chunk = body.subarray(offset, offset + budget)
    const frame = new Uint8Array(1 + chunk.length)
    frame[0] = FRAME_INPUT
    frame.set(chunk, 1)
    frames.push(frame)
  }
  return frames
}

/** Decodes one server frame, or null when it is malformed or of an unknown kind. */
export function decodeFrame(payload: ArrayBuffer | Uint8Array): PopupTerminalFrame | null {
  const bytes = payload instanceof Uint8Array ? payload : new Uint8Array(payload)
  if (bytes.length === 0) return null

  switch (bytes[0]) {
    case FRAME_OUTPUT:
      return { type: 'output', data: bytes.subarray(1) }
    case FRAME_EXIT: {
      try {
        const body = JSON.parse(decoder.decode(bytes.subarray(1))) as { reason?: string }
        return { type: 'exit', reason: body?.reason ?? '' }
      } catch {
        return { type: 'exit', reason: '' }
      }
    }
    default:
      return null
  }
}

function toTerminalState(term: Partial<PopupTerminalState>): PopupTerminalState {
  return {
    id: term.id ?? '',
    title: term.title ?? '',
    dir: term.dir ?? '',
    command: term.command ?? '',
    cols: term.cols ?? 0,
    rows: term.rows ?? 0,
  }
}

async function failure(response: Response): Promise<PopupTerminalRequestError> {
  try {
    const body = await response.json() as { message?: string; kind?: string }
    if (body?.message) return new PopupTerminalRequestError(body.message, body.kind ?? '')
  } catch {
    // fall through to the status line
  }
  return new PopupTerminalRequestError(`${response.status} ${response.statusText}`.trim(), '')
}
