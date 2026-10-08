// The terminal transport. Control actions are plain fetch calls against the
// loopback HTTP API; the data plane is a native WebSocket carrying the binary
// frames defined in cmd/desktop/internal/adapter/httpapi/terminal_stream.go:
//
//   server -> client
//     0x00 Output      [0x00][winLen u8][windowId][paneLen u8][paneId][raw bytes]
//     0x01 WindowEvent [0x01][JSON {kind, windowId, name, active, activePane, width, height, zoomed, layout}]
//     0x02 Lifecycle   [0x02][JSON {kind, windowId, message}]
//   client -> server
//     0x10 Input       [0x10][paneLen u8][paneId][raw bytes]
//     0x11 PasteChunk  [0x11][paneLen u8][paneId][raw bytes]
//     0x12 PasteCommit [0x12][paneLen u8][paneId]

import { Endpoint } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice'
import type { TerminalEndpoint } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

export type { TerminalEndpoint }

/** Carried as ?v=; the server rejects anything else before the upgrade. */
export const TERMINAL_WIRE_VERSION = '2'

/** The server's whole-frame cap on one client -> server message. */
export const MAX_INPUT_FRAME_BYTES = 4 << 10

const FRAME_OUTPUT = 0x00
const FRAME_WINDOW_EVENT = 0x01
const FRAME_LIFECYCLE = 0x02
const FRAME_INPUT = 0x10
const FRAME_PASTE_CHUNK = 0x11
const FRAME_PASTE_COMMIT = 0x12

/**
 * `layout-changed` covers a resize as well as a split, a closed pane and a
 * zoom: the window's size is its layout's root box.
 */
export type WindowEventKind = 'added' | 'closed' | 'renamed' | 'active-changed' | 'layout-changed'
/**
 * Mirrors `tmuxcc.LifecycleKind`, which is where these strings are minted —
 * adding one is an edit on both sides. `degraded` is the odd one: the stream
 * lives on, output was lost, and the repaint that re-establishes the panes is
 * already behind it on the wire.
 */
export type LifecycleKind = 'attached' | 'paused' | 'resumed' | 'exited' | 'error' | 'degraded'

/**
 * One cell of a window's pane tree, in cells of the window's grid. A leaf
 * names a pane; a node names a split and carries its cells, each with its own
 * box, so a pane is placed without walking the tree. tmux never nests a node
 * inside one of the same kind, so the one-cell border between two siblings
 * always belongs to their parent.
 */
export interface PaneLayout {
  paneId?: string
  /** `leftright` is cells side by side (tmux's split-window -h); `topbottom` is stacked (-v). Absent on a leaf. */
  split?: 'leftright' | 'topbottom'
  x: number
  y: number
  width: number
  height: number
  cells?: PaneLayout[]
}

/**
 * One tmux window. `width`/`height` are tmux's own size for it — whichever
 * attached client tmux's window-size option picked, which may be another
 * terminal entirely — and 0 when tmux has not reported one. Rendering at any
 * other size mangles the pane's cursor-addressed output. `layout` is where
 * each pane sits inside that grid, null until tmux has reported one; `zoomed`
 * says the active pane is drawn over the whole window while the layout still
 * records where it goes back to.
 */
export interface WindowState {
  windowId: string
  name: string
  active: boolean
  activePane: string
  width: number
  height: number
  zoomed: boolean
  layout: PaneLayout | null
}

/** Which way a split lays the new pane, in tmux's words: horizontal puts it to the right, vertical below. */
export type SplitDirection = 'horizontal' | 'vertical'
/** A neighbour of a pane, the way tmux's own select-pane -L/-R/-U/-D picks one. */
export type PaneDirection = 'left' | 'right' | 'up' | 'down'

export type TerminalFrame =
  | { type: 'output'; windowId: string; paneId: string; data: Uint8Array }
  | { type: 'window'; kind: WindowEventKind; state: WindowState }
  | { type: 'lifecycle'; kind: LifecycleKind; windowId: string; message: string }

/**
 * A control-plane failure carrying the core's own classification, so a caller
 * can tell a session that is not running (`not_found`) from tmux being
 * unusable (`unavailable`) without reading the message.
 */
export class TerminalRequestError extends Error {
  constructor(
    message: string,
    readonly kind: string,
  ) {
    super(message)
    this.name = 'TerminalRequestError'
  }
}

/** What a window has in front of it — see TerminalClient.windowForeground. */
export interface WindowForeground {
  running: boolean
  /** The foreground process's name; empty when nothing is running, or when the name could not be read. */
  command: string
}

export interface TerminalClient {
  /**
   * cols/rows are the opening size vote; 0x0 attaches without setting one.
   * Attaching never spawns: a session that is not running rejects with a
   * `not_found` TerminalRequestError, and start() is what creates it.
   */
  attach(slug: string, cols: number, rows: number): Promise<{ windows: WindowState[] }>
  /** Spawns the tmux session behind a slug, or reports it was already running. */
  start(slug: string): Promise<{ started: boolean; completed?: boolean }>
  /**
   * Kills the tmux session behind a slug — the terminal only; the hive session
   * and its checkout are untouched. A slug with no session answers killed:false.
   */
  kill(slug: string): Promise<{ killed: boolean }>
  /**
   * Lists several sessions' windows without attaching, keyed by slug. The
   * sidebar sweeps with this, so it takes the whole set: per slug, tmux
   * answered an unattached session by spawning twice, and the fan-out landed
   * on the frames the tree was painting in. A slug with no tmux session behind
   * it is absent from the result.
   */
  listWindows(slugs: string[]): Promise<Record<string, WindowState[]>>
  resize(slug: string, cols: number, rows: number): Promise<void>
  newWindow(slug: string): Promise<{ windowId: string }>
  newAgentWindow(slug: string, agent: string): Promise<{ windowId: string }>
  closeWindow(slug: string, windowId: string): Promise<void>
  /**
   * Reports whether a window is running anything a close would kill. `running`
   * is false only when every live pane in it is a shell waiting at its prompt;
   * a pane whose state could not be read answers true, so an unknown is never
   * mistaken for an idle one.
   */
  windowForeground(slug: string, windowId: string): Promise<WindowForeground>
  renameWindow(slug: string, windowId: string, name: string): Promise<void>
  /**
   * Moves a window to `position` in the session's window order — a 0-based
   * index into the resulting order, the way a drop on a tab strip means one —
   * and answers with the order tmux settled on. The order is tmux session
   * state, so the reply is authoritative and every other attached client sees
   * the move too.
   */
  moveWindow(slug: string, windowId: string, position: number): Promise<{ windows: WindowState[] }>
  selectWindow(slug: string, windowId: string): Promise<void>
  /**
   * Splits a pane and answers the new pane's id. The window's new layout, with
   * the new pane's first paint behind it, follows on the stream.
   */
  splitPane(slug: string, paneId: string, direction: SplitDirection): Promise<{ paneId: string }>
  /**
   * Makes a pane its window's active pane — or, with a direction, the
   * neighbour tmux picks from it. The stream announces the result.
   */
  selectPane(slug: string, paneId: string, direction?: PaneDirection): Promise<void>
  /** Kills one pane; the last pane of a window takes the window with it. */
  closePane(slug: string, paneId: string): Promise<void>
  /** Reports whether this pane is running anything a close would kill. */
  paneForeground(slug: string, paneId: string): Promise<WindowForeground>
  /**
   * Sets a pane's width and/or height in cells; 0 leaves that axis alone.
   * tmux moves the divider on the far side of the pane's cell, so a dragged
   * divider names the pane before it.
   */
  resizePane(slug: string, paneId: string, size: { width?: number; height?: number }): Promise<void>
  /** Toggles a pane between filling its window and its place in the layout. */
  zoomPane(slug: string, paneId: string): Promise<void>
  detach(slug: string): Promise<void>
  openStream(slug: string): WebSocket
}

const encoder = new TextEncoder()
const decoder = new TextDecoder()

/** Resolves the transport the webview was handed, or rejects with the reason. */
export async function getTerminalEndpoint(): Promise<TerminalEndpoint> {
  return await Endpoint()
}

export function createTerminalClient(endpoint: TerminalEndpoint): TerminalClient {
  async function post<T>(path: string, body: unknown): Promise<T | null> {
    const response = await fetch(endpoint.httpBaseURL + path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${endpoint.token}` },
      body: JSON.stringify(body),
    })
    if (!response.ok) throw await failure(response)
    if (response.status === 204) return null
    return (await response.json()) as T
  }

  return {
    async attach(slug, cols, rows) {
      const body = await post<{ windows: Partial<WindowState>[] | null }>('/api/terminal/attach', { slug, cols, rows })
      return { windows: (body?.windows ?? []).map(toWindowState) }
    },
    async start(slug) {
      const body = await post<{ started: boolean; completed?: boolean }>('/api/terminal/start', { slug })
      return { started: !!body?.started, ...(body?.completed ? { completed: true } : {}) }
    },
    async kill(slug) {
      const body = await post<{ killed: boolean }>('/api/terminal/kill', { slug })
      return { killed: !!body?.killed }
    },
    async listWindows(slugs) {
      const body = await post<{ sessions: Record<string, Partial<WindowState>[] | null> | null }>(
        '/api/terminal/windows/list',
        { slugs },
      )
      const listings: Record<string, WindowState[]> = {}
      for (const [slug, windows] of Object.entries(body?.sessions ?? {})) {
        listings[slug] = (windows ?? []).map(toWindowState)
      }
      return listings
    },
    async resize(slug, cols, rows) {
      await post('/api/terminal/resize', { slug, cols, rows })
    },
    async newWindow(slug) {
      const body = await post<{ windowId: string }>('/api/terminal/windows/new', { slug })
      return { windowId: body?.windowId ?? '' }
    },
    async newAgentWindow(slug, agent) {
      const body = await post<{ windowId: string }>('/api/terminal/windows/agent', { slug, agent })
      return { windowId: body?.windowId ?? '' }
    },
    async closeWindow(slug, windowId) {
      await post('/api/terminal/windows/close', { slug, windowId })
    },
    async windowForeground(slug, windowId) {
      const body = await post<Partial<WindowForeground>>('/api/terminal/windows/foreground', { slug, windowId })
      // An answer that carries no verdict is an unknown, and an unknown is
      // something running: the caller kills the window on the strength of it.
      return { running: body?.running ?? true, command: body?.command ?? '' }
    },
    async renameWindow(slug, windowId, name) {
      await post('/api/terminal/windows/rename', { slug, windowId, name })
    },
    async moveWindow(slug, windowId, position) {
      const body = await post<{ windows: Partial<WindowState>[] | null }>('/api/terminal/windows/move', {
        slug,
        windowId,
        position,
      })
      return { windows: (body?.windows ?? []).map(toWindowState) }
    },
    async selectWindow(slug, windowId) {
      await post('/api/terminal/windows/select', { slug, windowId })
    },
    async splitPane(slug, paneId, direction) {
      const body = await post<{ paneId: string }>('/api/terminal/panes/split', { slug, paneId, direction })
      return { paneId: body?.paneId ?? '' }
    },
    async selectPane(slug, paneId, direction) {
      await post('/api/terminal/panes/select', { slug, paneId, direction: direction ?? '' })
    },
    async closePane(slug, paneId) {
      await post('/api/terminal/panes/close', { slug, paneId })
    },
    async paneForeground(slug, paneId) {
      const body = await post<Partial<WindowForeground>>('/api/terminal/panes/foreground', { slug, paneId })
      return { running: body?.running ?? true, command: body?.command ?? '' }
    },
    async resizePane(slug, paneId, size) {
      await post('/api/terminal/panes/resize', { slug, paneId, width: size.width ?? 0, height: size.height ?? 0 })
    },
    async zoomPane(slug, paneId) {
      await post('/api/terminal/panes/zoom', { slug, paneId })
    },
    async detach(slug) {
      await post('/api/terminal/detach', { slug })
    },
    openStream(slug) {
      const socket = new WebSocket(streamURL(endpoint, slug))
      socket.binaryType = 'arraybuffer'
      return socket
    },
  }
}

/**
 * Builds the data-plane URL. The bearer token cannot ride a header on a browser
 * handshake, so it goes in the query string the server also accepts.
 */
function streamURL(endpoint: TerminalEndpoint, slug: string): string {
  const url = new URL(endpoint.wsURL)
  url.searchParams.set('slug', slug)
  url.searchParams.set('token', endpoint.token)
  url.searchParams.set('v', TERMINAL_WIRE_VERSION)
  return url.toString()
}

/**
 * Splits keystrokes into input frames no larger than the server's cap. Bytes
 * are reassembled in order on the pane, so a split inside a multi-byte
 * sequence is harmless. The frame names the pane rather than its window: the
 * keystrokes that follow a click into a pane go out while the select-pane it
 * caused is still in flight, and they must land where the user is typing.
 */
export function encodeInputFrames(paneId: string, data: string | Uint8Array): Uint8Array<ArrayBuffer>[] {
  return chunkFrames(FRAME_INPUT, paneId, typeof data === 'string' ? encoder.encode(data) : data)
}

/**
 * Frames one paste: its bytes, then a commit the server pastes on. Pasted text
 * is not keystrokes — the server hands it to tmux, the only side that knows
 * whether the pane's program asked for bracketed paste, so a multi-line paste
 * reaches an agent as one paste rather than one submission per line.
 */
export function encodePasteFrames(paneId: string, text: string): Uint8Array<ArrayBuffer>[] {
  // tmux writes one \r per \n in the buffer, so a CRLF would arrive as two.
  const body = encoder.encode(text.replace(/\r\n?/g, '\n'))
  if (body.length === 0) return []

  const id = encoder.encode(paneId)
  const commit = new Uint8Array(2 + id.length)
  commit[0] = FRAME_PASTE_COMMIT
  commit[1] = id.length
  commit.set(id, 2)
  return [...chunkFrames(FRAME_PASTE_CHUNK, paneId, body), commit]
}

function chunkFrames(kind: number, paneId: string, body: Uint8Array): Uint8Array<ArrayBuffer>[] {
  const id = encoder.encode(paneId)
  const header = 2 + id.length
  const budget = MAX_INPUT_FRAME_BYTES - header
  if (budget <= 0) throw new Error('terminal pane id is too long to frame')

  const frames: Uint8Array<ArrayBuffer>[] = []
  for (let offset = 0; offset < body.length; offset += budget) {
    const chunk = body.subarray(offset, offset + budget)
    const frame = new Uint8Array(header + chunk.length)
    frame[0] = kind
    frame[1] = id.length
    frame.set(id, 2)
    frame.set(chunk, header)
    frames.push(frame)
  }
  return frames
}

/** Decodes one server frame, or null when it is malformed or of an unknown kind. */
export function decodeFrame(payload: ArrayBuffer | Uint8Array): TerminalFrame | null {
  const bytes = payload instanceof Uint8Array ? payload : new Uint8Array(payload)
  if (bytes.length === 0) return null

  switch (bytes[0]) {
    case FRAME_OUTPUT: {
      const window = readID(bytes, 1)
      if (!window) return null
      const pane = readID(bytes, window.next)
      if (!pane) return null
      return { type: 'output', windowId: window.id, paneId: pane.id, data: bytes.subarray(pane.next) }
    }
    case FRAME_WINDOW_EVENT: {
      const payload = readJSON<WindowState & { kind: WindowEventKind }>(bytes)
      if (!payload?.kind) return null
      return { type: 'window', kind: payload.kind, state: toWindowState(payload) }
    }
    case FRAME_LIFECYCLE: {
      const payload = readJSON<{ kind: LifecycleKind; windowId: string; message: string }>(bytes)
      if (!payload?.kind) return null
      return { type: 'lifecycle', kind: payload.kind, windowId: payload.windowId ?? '', message: payload.message ?? '' }
    }
    default:
      return null
  }
}

function toWindowState(window: Partial<WindowState>): WindowState {
  return {
    windowId: window.windowId ?? '',
    name: window.name ?? '',
    active: !!window.active,
    activePane: window.activePane ?? '',
    width: window.width ?? 0,
    height: window.height ?? 0,
    zoomed: !!window.zoomed,
    layout: window.layout ?? null,
  }
}

function readID(bytes: Uint8Array, offset: number): { id: string; next: number } | null {
  if (offset >= bytes.length) return null
  const size = bytes[offset]
  const start = offset + 1
  if (start + size > bytes.length) return null
  return { id: decoder.decode(bytes.subarray(start, start + size)), next: start + size }
}

function readJSON<T>(bytes: Uint8Array): T | null {
  try {
    return JSON.parse(decoder.decode(bytes.subarray(1))) as T
  } catch {
    return null
  }
}

async function failure(response: Response): Promise<TerminalRequestError> {
  try {
    const body = (await response.json()) as { message?: string; kind?: string }
    if (body?.message) return new TerminalRequestError(body.message, body.kind ?? '')
  } catch {
    // fall through to the status line
  }
  return new TerminalRequestError(`${response.status} ${response.statusText}`.trim(), '')
}
