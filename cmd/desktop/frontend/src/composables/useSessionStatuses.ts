import { ref, type Ref } from 'vue'
import { SessionStatuses } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import type { SessionStatus } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const DEFAULT_POLL_INTERVAL_MS = 1500

const statuses = ref<Record<string, SessionStatus>>({})
const pollIntervalMs = ref(DEFAULT_POLL_INTERVAL_MS)
// Whether a poll has ever finished. An empty status map cannot answer that, and
// a view that narrows on liveness has to tell "nothing is running" from "we have
// not asked yet". Set whatever the poll returned: a probe that keeps failing
// must not leave such a view waiting forever.
const loaded = ref(false)
let requestSequence = 0
let pollGeneration = 0
let pollTimer: ReturnType<typeof setTimeout> | undefined
let activeRequest: ReturnType<typeof SessionStatuses> | undefined
let pollingRequested = false
let visibilityWatched = false

function cancelActiveRequest(): void {
  const request = activeRequest
  activeRequest = undefined
  if (request && typeof request.cancel === 'function') void request.cancel()
}

async function reload(): Promise<void> {
  const sequence = ++requestSequence
  cancelActiveRequest()
  let request: ReturnType<typeof SessionStatuses> | undefined
  try {
    request = SessionStatuses()
    activeRequest = request
    const snapshot = await request
    if (sequence !== requestSequence) return
    statuses.value = Object.fromEntries((snapshot.items ?? []).map((status) => [status.sessionId, status]))
    if (snapshot.pollIntervalMs > 0) pollIntervalMs.value = snapshot.pollIntervalMs
  } catch {
    // A transient tmux probe failure must not erase the last status the user saw.
  } finally {
    if (request && activeRequest === request) activeRequest = undefined
    if (sequence === requestSequence) loaded.value = true
  }
}

async function poll(generation: number): Promise<void> {
  await reload()
  if (generation !== pollGeneration || document.hidden) return
  pollTimer = setTimeout(() => {
    void poll(generation)
  }, pollIntervalMs.value)
}

function restartPoll(): void {
  clearTimeout(pollTimer)
  const generation = ++pollGeneration
  void poll(generation)
}

// Each poll walks the process table and spawns tmux, and nobody sees the dots
// of a hidden window, so the loop parks until the window shows again.
function onVisibilityChange(): void {
  if (pollingRequested && !document.hidden) restartPoll()
}

function startPolling(): void {
  pollingRequested = true
  if (!visibilityWatched) {
    visibilityWatched = true
    document.addEventListener('visibilitychange', onVisibilityChange)
  }
  restartPoll()
}

function stopPolling(): void {
  pollingRequested = false
  ++pollGeneration
  ++requestSequence
  clearTimeout(pollTimer)
  pollTimer = undefined
  cancelActiveRequest()
}

export function useSessionStatuses(): {
  statuses: Ref<Record<string, SessionStatus>>
  loaded: Ref<boolean>
  pollIntervalMs: Ref<number>
  reload: () => Promise<void>
  startPolling: () => void
  stopPolling: () => void
} {
  return { statuses, loaded, pollIntervalMs, reload, startPolling, stopPolling }
}

export function resetSessionStatusesForTests(): void {
  stopPolling()
  statuses.value = {}
  loaded.value = false
  pollIntervalMs.value = DEFAULT_POLL_INTERVAL_MS
}
