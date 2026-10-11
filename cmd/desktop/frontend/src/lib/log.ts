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

const RANK: Record<LogLevel, number> = { debug: 0, info: 1, warn: 2, error: 3 }
const SILENT = 4

const FLUSH_MS = 1000
const ERROR_FLUSH_MS = 250
const MAX_BUFFERED = 200

let threshold: number | null = null
let levelRequest: Promise<number> | null = null

let buffer: ClientLogEntry[] = []
// An error thrown on every frame costs one line per flush.
let pending = new Map<string, ClientLogEntry>()
let dropped = 0
let timer: ReturnType<typeof setTimeout> | null = null
let timerDue = 0

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

function backendThreshold(): Promise<number> {
  if (threshold !== null) return Promise.resolve(threshold)
  levelRequest ??= Promise.resolve()
    .then(() => BackendLevel())
    .then(rankOf, () => RANK.info)
    .then((rank) => (threshold = rank))
  return levelRequest
}

async function flush(): Promise<void> {
  if (timer !== null) {
    clearTimeout(timer)
    timer = null
  }
  if (buffer.length === 0 && dropped === 0) return

  const min = await backendThreshold()
  const batch = buffer.filter((entry) => rankOf(entry.level) >= min)
  if (dropped > 0) {
    batch.push(entry('warn', '', 'webview log buffer overflowed; oldest entries dropped', undefined, { dropped }))
  }
  buffer = []
  pending = new Map()
  dropped = 0
  if (batch.length === 0) return

  try {
    await Write(batch)
  } catch {
    // Never logged: a failure to deliver logs reported as a log would loop.
  }
}

function schedule(delay: number): void {
  const due = Date.now() + delay
  if (timer !== null) {
    if (timerDue <= due) return
    clearTimeout(timer)
  }
  timerDue = due
  timer = setTimeout(() => void flush(), delay)
}

function currentWindow(): string {
  return new URLSearchParams(window.location.search).has('diagnostics') ? 'diagnostics' : 'main'
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

function entry(level: LogLevel, name: string, message: string, detail: unknown, fields?: LogFields): ClientLogEntry {
  const described = describeDetail(detail)
  const merged = described.fields || fields ? { ...described.fields, ...fields } : undefined
  return {
    atUnixMs: Date.now(),
    level,
    name,
    message,
    error: described.error,
    kind: described.kind,
    stack: described.stack,
    window: currentWindow(),
    route: window.location.hash || window.location.pathname,
    count: 1,
    fields: merged,
  }
}

function dedupeKey(e: ClientLogEntry): string {
  return [e.level, e.name, e.message, e.error, e.stack].join('\u0000')
}

function enqueue(next: ClientLogEntry): void {
  if (threshold !== null && rankOf(next.level) < threshold) return

  const key = dedupeKey(next)
  const existing = pending.get(key)
  if (existing) {
    existing.count = (existing.count ?? 1) + 1
  } else {
    if (buffer.length >= MAX_BUFFERED) {
      const oldest = buffer.shift()
      if (oldest) pending.delete(dedupeKey(oldest))
      dropped++
    }
    buffer.push(next)
    pending.set(key, next)
  }
  schedule(next.level === 'error' ? ERROR_FLUSH_MS : FLUSH_MS)
}

function echo(level: LogLevel, message: string, detail: unknown, fields?: LogFields): void {
  const args: unknown[] = [message]
  if (detail !== undefined) args.push(detail)
  if (fields !== undefined) args.push(fields)
  // eslint-disable-next-line no-console -- the one place the webview writes to its console
  console[level](...args)
}

// Specs assert on behaviour, not console output.
const echoToConsole = import.meta.env.MODE !== 'test'

function emit(level: LogLevel, name: string, message: string, detail: unknown, fields?: LogFields, toConsole = true) {
  if (toConsole && echoToConsole) echo(level, message, detail, fields)
  try {
    enqueue(entry(level, name, message, detail, fields))
  } catch {
    // A logging call never throws into its caller.
  }
}

/** Lines carry `cmp=webview.<name>`; name it after the module ('feed-state'). */
export function createLogger(name = ''): Logger {
  return {
    debug: (message, detail, fields) => emit('debug', name, message, detail, fields),
    info: (message, detail, fields) => emit('info', name, message, detail, fields),
    warn: (message, detail, fields) => emit('warn', name, message, detail, fields),
    error: (message, detail, fields) => emit('error', name, message, detail, fields),
  }
}

let captureInstalled = false

// Not echoed: the browser already prints uncaught errors.
export function installErrorCapture(target: Window = window): void {
  if (captureInstalled) return
  captureInstalled = true
  target.addEventListener('error', (event: ErrorEvent) => {
    emit(
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
    emit('error', 'uncaught', 'unhandled promise rejection', event.reason, undefined, false)
  })
  target.addEventListener('pagehide', () => void flush())
}

// Echoed, because Vue stops printing errors once a handler is set.
export function captureVueErrors(app: App): void {
  app.config.errorHandler = (err, instance, info) => {
    const component = instance?.$options.name ?? instance?.$options.__name
    emit('error', 'vue', 'error caught by Vue', err, { component, hook: info })
  }
}

export function flushLogs(): Promise<void> {
  return flush()
}

export function resetLogForTests(): void {
  if (timer !== null) clearTimeout(timer)
  timer = null
  timerDue = 0
  threshold = null
  levelRequest = null
  buffer = []
  pending = new Map()
  dropped = 0
}
