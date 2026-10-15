import { createLogger } from './log'
import {
  Connect,
  Disconnect,
  Request,
  OpenStream,
  ReadStream,
  WriteStream,
  CloseStream,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/remoteservice'
import type { TerminalEndpoint } from './terminalClient'

export interface TerminalStream {
  readyState: number
  binaryType: BinaryType
  onopen: ((event: Event) => void) | null
  onmessage: ((event: MessageEvent) => void) | null
  onclose: ((event: CloseEvent) => void) | null
  onerror: ((event: Event) => void) | null
  send(data: ArrayBuffer | ArrayBufferView<ArrayBuffer>): void
  close(): void
}
const log = createLogger('remote-transport')
const prefix = 'hive-remote://'
export function managedID(endpoint: TerminalEndpoint): string | null {
  return endpoint.httpBaseURL.startsWith(prefix) ? endpoint.httpBaseURL.slice(prefix.length) : null
}
export async function connectManaged(host: string, connectionFile: string): Promise<TerminalEndpoint> {
  const id = await Connect({ host, connectionFile })
  return { httpBaseURL: prefix + id, wsURL: '', token: '' }
}
export async function disconnectManaged(endpoint: TerminalEndpoint): Promise<void> {
  const id = managedID(endpoint)
  if (id) await Disconnect(id)
}
export async function endpointFetch(endpoint: TerminalEndpoint, path: string, init: RequestInit): Promise<Response> {
  const id = managedID(endpoint)
  if (!id) return fetch(endpoint.httpBaseURL + path, init)
  const response = await Request(id, path, typeof init.body === 'string' ? init.body : '{}')
  return new Response(response.status === 204 ? null : response.body, { status: response.status })
}

export function openManagedStream(id: string, slug: string): TerminalStream {
  let stream = ''
  let stopped = false
  let sending = false
  let queuedBytes = 0
  const queue: Uint8Array[] = []
  const socket: TerminalStream = {
    readyState: 0,
    binaryType: 'arraybuffer',
    onopen: null,
    onmessage: null,
    onclose: null,
    onerror: null,
    send(data) {
      if (socket.readyState !== 1) return
      const bytes =
        data instanceof ArrayBuffer
          ? new Uint8Array(data)
          : new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
      if (queuedBytes + bytes.byteLength > 8 * 1024 * 1024) {
        fail()
        return
      }
      queue.push(bytes.slice())
      queuedBytes += bytes.byteLength
      void sendQueued()
    },
    close() {
      if (stopped) return
      stopped = true
      socket.readyState = 3
      queue.length = 0
      queuedBytes = 0
      if (stream)
        void CloseStream(id, stream).catch((error: unknown) => log.warn('Remote stream cleanup failed', error))
    },
  }
  function fail(): void {
    if (stopped) return
    socket.close()
    socket.onerror?.(new Event('error'))
    socket.onclose?.(new CloseEvent('close'))
  }
  async function sendQueued(): Promise<void> {
    if (sending || !stream) return
    sending = true
    try {
      while (!stopped && queue.length) {
        const data = queue.shift()!
        queuedBytes -= data.byteLength
        await WriteStream(id, stream, btoa(String.fromCharCode(...data)))
      }
    } catch {
      fail()
    } finally {
      sending = false
    }
  }
  async function receive(): Promise<void> {
    try {
      stream = await OpenStream(id, slug)
      if (stopped) {
        await CloseStream(id, stream)
        return
      }
      socket.readyState = 1
      socket.onopen?.(new Event('open'))
      while (!stopped) {
        const batch = await ReadStream(id, stream)
        if (stopped) return
        for (const frame of batch.frames ?? []) {
          const bytes = Uint8Array.from(atob(frame), (character) => character.charCodeAt(0))
          socket.onmessage?.(new MessageEvent('message', { data: bytes.buffer }))
          if (stopped) return
        }
        if (batch.ended) {
          socket.close()
          socket.onclose?.(new CloseEvent('close'))
          return
        }
      }
    } catch {
      fail()
    }
  }
  void receive()
  return socket
}
