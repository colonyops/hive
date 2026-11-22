import { useDocumentVisibility, useIntervalFn, useThrottleFn, useTimeoutFn } from '@vueuse/core'
import { onScopeDispose, ref, shallowReadonly, shallowRef, watch, type Ref } from 'vue'
import { api, ApiError, describeApiError, encodePath } from '../api'
import type {
  Anchor,
  DocumentEvent,
  LineRange,
  PresenceSnapshot,
  PresenceViewer,
  StreamHello,
  Thread,
  Viewer,
} from '../types'
import { useViewerIdentity } from './identity'

export type CollabTarget = { kind: 'owner'; apiPrefix: string; path: string } | { kind: 'link'; slug: string }

const heartbeatEvery = 15_000
const selectionThrottle = 500
const reconnectAfter = 5_000

/** The routes one target's threads live under. */
export function collabRoutes(target: CollabTarget) {
  if (target.kind === 'owner') {
    const doc = encodePath(target.path)
    return {
      threads: `${target.apiPrefix}/threads/${doc}`,
      create: `${target.apiPrefix}/threads/${doc}`,
      events: `${target.apiPrefix}/events/${doc}`,
      thread: (id: string) => `/api/v1/threads/${encodeURIComponent(id)}`,
      comment: (id: string) => `/api/v1/comments/${encodeURIComponent(id)}`,
    }
  }
  const slug = encodeURIComponent(target.slug)
  return {
    threads: `/api/v1/public/${slug}/threads`,
    create: `/api/v1/links/${slug}/threads`,
    events: `/api/v1/public/${slug}/events`,
    thread: (id: string) => `/api/v1/links/${slug}/threads/${encodeURIComponent(id)}`,
    comment: (id: string) => `/api/v1/links/${slug}/comments/${encodeURIComponent(id)}`,
  }
}

export interface CollabOptions {
  /** The sha of the body on screen; a new thread is checked against it. */
  renderedSha: Readonly<Ref<string>>
  /**
   * Fetches the body again: on every stream open and on a document event,
   * with the version the server has. Without one, after a version conflict,
   * the body must be fetched.
   */
  reloadDocument: (sha?: string) => Promise<void>
  /** The document was deleted or its link revoked. */
  onClosed: () => void
}

function errorFor(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.status === 409) return 'The document changed. Select the text again.'
  return describeApiError(error, fallback)
}

/**
 * Threads and presence for one document. The stream only signals; content
 * always comes from REST, and every stream open re-reads, so a reconnect
 * catches up on whatever it missed.
 */
export function useDocumentCollab(target: CollabTarget, opts: CollabOptions) {
  const routes = collabRoutes(target)
  const identity = target.kind === 'link' ? useViewerIdentity() : null

  const threads = ref<Thread[]>([])
  const viewer = ref<Viewer | null>(null)
  const viewers = ref<PresenceViewer[]>([])
  const connected = ref(false)
  const error = ref<string | null>(null)

  let token: string | null = null
  const selection = shallowRef<LineRange | null>(null)
  let source: EventSource | null = null
  let closed = false
  let loadSeq = 0

  async function reloadThreads(): Promise<void> {
    // Loads overlap when events arrive close together; only the latest may land.
    const seq = ++loadSeq
    try {
      const list = await api.threads(routes.threads)
      if (seq !== loadSeq) return
      threads.value = list.threads
      viewer.value = list.viewer
      error.value = null
    } catch (e) {
      if (seq !== loadSeq) return
      error.value = describeApiError(e, 'Could not load comments.')
    }
  }

  function sendPresence(): void {
    if (!token) return
    api
      .presence({
        token,
        name: identity?.name.value,
        color: identity?.color ?? 0,
        selection: selection.value ?? undefined,
      })
      .catch(() => {
        // A heartbeat after the stream closed finds no viewer; the next hello issues a new token.
      })
  }
  const sendSelection = useThrottleFn(sendPresence, selectionThrottle, true)

  const visibility = useDocumentVisibility()
  const heartbeat = useIntervalFn(sendPresence, heartbeatEvery, { immediate: false })
  watch(visibility, (state) => {
    if (state === 'visible' && connected.value) {
      heartbeat.resume()
      sendPresence()
    } else heartbeat.pause()
  })

  const reconnect = useTimeoutFn(() => open(), reconnectAfter, { immediate: false })

  function open(): void {
    if (closed) return
    source?.close()
    const es = new EventSource(routes.events)
    source = es
    es.addEventListener('hello', (event) => {
      const hello = JSON.parse((event as MessageEvent<string>).data) as StreamHello
      token = hello.token
      connected.value = true
      if (visibility.value === 'visible') heartbeat.resume()
      sendPresence()
      void reloadThreads()
      void opts.reloadDocument(hello.sha256)
    })
    es.addEventListener('presence', (event) => {
      viewers.value = (JSON.parse((event as MessageEvent<string>).data) as PresenceSnapshot).viewers
    })
    es.addEventListener('threads', () => void reloadThreads())
    es.addEventListener('document', (event) => {
      void opts.reloadDocument((JSON.parse((event as MessageEvent<string>).data) as DocumentEvent).sha256)
      void reloadThreads()
    })
    es.addEventListener('closed', () => {
      stop()
      opts.onClosed()
    })
    es.addEventListener('error', () => {
      connected.value = false
      heartbeat.pause()
      token = null
      // The browser retries a dropped stream itself; a refused one it gives up on.
      if (es.readyState === EventSource.CLOSED && source === es) reconnect.start()
    })
  }

  function stop(): void {
    closed = true
    connected.value = false
    heartbeat.pause()
    reconnect.stop()
    source?.close()
    source = null
    token = null
  }

  open()
  onScopeDispose(stop)

  async function run<T>(action: () => Promise<T>, fallback: string): Promise<string | null> {
    try {
      await action()
      await reloadThreads()
      return null
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) void opts.reloadDocument()
      return errorFor(e, fallback)
    }
  }

  return {
    threads: shallowReadonly(threads),
    viewer: shallowReadonly(viewer),
    viewers: shallowReadonly(viewers),
    connected: shallowReadonly(connected),
    error: shallowReadonly(error),
    anonymousName: identity?.name ?? null,
    renameAnonymous: identity ? (name: string) => (identity.rename(name), sendPresence()) : null,
    setSelection(next: LineRange | null): void {
      const same = next?.start_line === selection.value?.start_line && next?.end_line === selection.value?.end_line
      if (same) return
      selection.value = next
      void sendSelection()
    },
    createThread: (anchor: Anchor | null, body: string) =>
      run(() => api.createThread(routes.create, opts.renderedSha.value, anchor, body), 'Could not post the comment.'),
    reply: (threadId: string, body: string) =>
      run(() => api.reply(`${routes.thread(threadId)}/comments`, body), 'Could not post the reply.'),
    setResolved: (threadId: string, resolved: boolean) =>
      run(() => api.setResolved(routes.thread(threadId), resolved), 'Could not update the thread.'),
    editComment: (id: string, body: string) =>
      run(() => api.editComment(routes.comment(id), body), 'Could not save the comment.'),
    deleteComment: (id: string) => run(() => api.deleteComment(routes.comment(id)), 'Could not delete the comment.'),
  }
}

export type DocumentCollab = ReturnType<typeof useDocumentCollab>
