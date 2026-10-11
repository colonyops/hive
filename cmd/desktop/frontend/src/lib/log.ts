import type { App } from 'vue'
import {
  Level as BackendLevel,
  Write,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/clientlogservice'
import type { ClientLogEntry } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { appErrorKind, appErrorMessage } from './appError'

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'
export type LogFields = Record<string, unknown>

/** An Error `detail` becomes error, stack and kind fields; any other non-string becomes a `detail` field. */
type LogMethod = (message: string, detail?: unknown, fields?: LogFields) => void

export interface Logger {
  debug: LogMethod
  info: LogMethod
  warn: LogMethod
  error: LogMethod
}

export interface LogTransport {
  level: () => Promise<string>
  write: (entries: ClientLogEntry[]) => Promise<void>
}

type LogLocation = Pick<Location, 'search' | 'hash' | 'pathname'>

export interface LogSinkOptions {
  transport: LogTransport
  echoToConsole?: boolean
  now?: () => number
  location?: () => LogLocation
}

const RANK: Record<LogLevel, number> = { debug: 0, info: 1, warn: 2, error: 3 }
const SILENT = 4

const FLUSH_MS = 1000
const ERROR_FLUSH_MS = 250
const MAX_BUFFERED = 200

function rankOf(level: string): number {
  switch (level) {
    case 'trace':
    case 'debug':
      return RANK.debug
    case 'warn':
      return RANK.warn
    case 'error':
    case 'fatal':
    case 'panic':
      return RANK.error
    case 'disabled':
      return SILENT
    default:
      return RANK.info
  }
}

function describeDetail(detail: unknown): Pick<ClientLogEntry, 'error' | 'kind' | 'stack'> & { fields?: LogFields } {
  if (detail === undefined || detail === null) return {}
  if (detail instanceof Error) {
    return {
      error: appErrorMessage(detail) || detail.message,
      kind: appErrorKind(detail) ?? undefined,
      stack: detail.stack,
    }
  }
  if (typeof detail === 'string') return { error: detail }
  if (typeof detail === 'object') {
    const message = appErrorMessage(detail)
    if (message) return { error: message, kind: appErrorKind(detail) ?? undefined }
    return { fields: { detail } }
  }
  if (typeof detail === 'function') return { error: detail.name || 'function' }
  return { error: (detail as number | boolean | bigint | symbol).toString() }
}

function dedupeKey(e: ClientLogEntry): string {
  return [e.level, e.name, e.message, e.error, e.stack].join('\u0000')
}

function echo(level: LogLevel, message: string, detail: unknown, fields?: LogFields): void {
  const args: unknown[] = [message]
  if (detail !== undefined) args.push(detail)
  if (fields !== undefined) args.push(fields)
  // eslint-disable-next-line no-console -- the one place the webview writes to its console
  console[level](...args)
}

export class LogSink {
  private readonly transport: LogTransport
  private readonly echoToConsole: boolean
  private readonly now: () => number
  private readonly location: () => LogLocation

  private threshold: number | null = null
  private levelRequest: Promise<number> | null = null
  private buffer: ClientLogEntry[] = []
  // An error thrown on every frame costs one line per flush.
  private pending = new Map<string, ClientLogEntry>()
  private dropped = 0
  private timer: ReturnType<typeof setTimeout> | null = null
  private timerDue = 0
  private captureInstalled = false

  constructor(options: LogSinkOptions) {
    this.transport = options.transport
    this.echoToConsole = options.echoToConsole ?? true
    this.now = options.now ?? Date.now
    this.location = options.location ?? (() => window.location)
  }

  /** Lines carry `cmp=webview.<name>`; name it after the module ('feed-state'). */
  logger(name = ''): Logger {
    return {
      debug: (message, detail, fields) => this.emit('debug', name, message, detail, fields),
      info: (message, detail, fields) => this.emit('info', name, message, detail, fields),
      warn: (message, detail, fields) => this.emit('warn', name, message, detail, fields),
      error: (message, detail, fields) => this.emit('error', name, message, detail, fields),
    }
  }

  // Not echoed: the browser already prints uncaught errors.
  installErrorCapture(target: Window): void {
    if (this.captureInstalled) return
    this.captureInstalled = true
    target.addEventListener('error', (event: ErrorEvent) => {
      this.emit(
        'error',
        'uncaught',
        'uncaught error',
        event.error ?? event.message,
        {
          source: event.filename || undefined,
          line: event.lineno || undefined,
          column: event.colno || undefined,
        },
        false,
      )
    })
    target.addEventListener('unhandledrejection', (event: PromiseRejectionEvent) => {
      this.emit('error', 'uncaught', 'unhandled promise rejection', event.reason, undefined, false)
    })
    target.addEventListener('pagehide', () => void this.flush())
  }

  // Echoed, because Vue stops printing errors once a handler is set.
  captureVueErrors(app: App): void {
    app.config.errorHandler = (err, instance, info) => {
      const component = instance?.$options.name ?? instance?.$options.__name
      this.emit('error', 'vue', 'error caught by Vue', err, { component, hook: info })
    }
  }

  async flush(): Promise<void> {
    if (this.timer !== null) {
      clearTimeout(this.timer)
      this.timer = null
    }
    if (this.buffer.length === 0 && this.dropped === 0) return

    const min = await this.backendThreshold()
    const batch = this.buffer.filter((entry) => rankOf(entry.level) >= min)
    if (this.dropped > 0) {
      batch.push(
        this.entry('warn', '', 'webview log buffer overflowed; oldest entries dropped', undefined, {
          dropped: this.dropped,
        }),
      )
    }
    this.buffer = []
    this.pending = new Map()
    this.dropped = 0
    if (batch.length === 0) return

    try {
      await this.transport.write(batch)
    } catch {
      // Never logged: a failure to deliver logs reported as a log would loop.
    }
  }

  private backendThreshold(): Promise<number> {
    if (this.threshold !== null) return Promise.resolve(this.threshold)
    this.levelRequest ??= Promise.resolve()
      .then(() => this.transport.level())
      .then(rankOf, () => RANK.info)
      .then((rank) => (this.threshold = rank))
    return this.levelRequest
  }

  private schedule(delay: number): void {
    const due = this.now() + delay
    if (this.timer !== null) {
      if (this.timerDue <= due) return
      clearTimeout(this.timer)
    }
    this.timerDue = due
    this.timer = setTimeout(() => void this.flush(), delay)
  }

  private entry(level: LogLevel, name: string, message: string, detail: unknown, fields?: LogFields): ClientLogEntry {
    const described = describeDetail(detail)
    const merged = described.fields || fields ? { ...described.fields, ...fields } : undefined
    const location = this.location()
    return {
      atUnixMs: this.now(),
      level,
      name,
      message,
      error: described.error,
      kind: described.kind,
      stack: described.stack,
      window: new URLSearchParams(location.search).has('diagnostics') ? 'diagnostics' : 'main',
      route: location.hash || location.pathname,
      count: 1,
      fields: merged,
    }
  }

  private enqueue(next: ClientLogEntry): void {
    if (this.threshold !== null && rankOf(next.level) < this.threshold) return

    const key = dedupeKey(next)
    const existing = this.pending.get(key)
    if (existing) {
      existing.count = (existing.count ?? 1) + 1
    } else {
      if (this.buffer.length >= MAX_BUFFERED) {
        const oldest = this.buffer.shift()
        if (oldest) this.pending.delete(dedupeKey(oldest))
        this.dropped++
      }
      this.buffer.push(next)
      this.pending.set(key, next)
    }
    this.schedule(next.level === 'error' ? ERROR_FLUSH_MS : FLUSH_MS)
  }

  private emit(level: LogLevel, name: string, message: string, detail: unknown, fields?: LogFields, toConsole = true) {
    if (toConsole && this.echoToConsole) echo(level, message, detail, fields)
    try {
      this.enqueue(this.entry(level, name, message, detail, fields))
    } catch {
      // A logging call never throws into its caller.
    }
  }
}

const sink = new LogSink({
  transport: { level: BackendLevel, write: Write },
  // Specs assert on behaviour, not console output.
  echoToConsole: import.meta.env.MODE !== 'test',
})

export function createLogger(name = ''): Logger {
  return sink.logger(name)
}

export function installErrorCapture(target: Window = window): void {
  sink.installErrorCapture(target)
}

export function captureVueErrors(app: App): void {
  sink.captureVueErrors(app)
}
