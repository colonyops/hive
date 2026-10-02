import { computed, onMounted, ref } from 'vue'
import {
  CancelDeviceFlow,
  Disconnect,
  SetToken,
  StartDeviceFlow,
  Status,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/githubservice'
import type { ConnectionStatus, DeviceFlowInfo } from '../types/github'
import { useWailsEvent } from './useWailsEvent'

export type ConnectCard = 'idle' | 'device' | 'token'

/** The provider this composable connects. */
const PROVIDER = 'github'

export function useGitHubConnection() {
  // null until the first Status() resolves, so the app can hold a loading
  // frame instead of flashing the connect cards at a connected user.
  const status = ref<ConnectionStatus | null>(null)
  const deviceFlow = ref<DeviceFlowInfo | null>(null)
  const card = ref<ConnectCard>('idle')
  // Errors from the active card's action; cleared on card switches.
  const actionError = ref<string | null>(null)
  // Backend-pushed failures ride ConnectionStatus.Message (connection:updated
  // carries only the provider); show them on the idle card unless a local
  // action error is fresher.
  const error = computed(() => {
    if (actionError.value) return actionError.value
    const current = status.value
    if (card.value === 'idle' && current && current.state !== 'connected' && current.message) return current.message
    return null
  })
  const busy = ref(false)
  const connected = computed(() => status.value?.state === 'connected')

  async function reload() {
    try {
      const next = await Status()
      status.value = next
      // A failure push while the device card waits means the flow died
      // (denied, expired, validation failed): fall back to the start card so
      // a dead user code is not left on screen.
      if (card.value === 'device' && next.state !== 'connected' && next.message) {
        card.value = 'idle'
        deviceFlow.value = null
      }
    } catch (err) {
      console.warn('Unable to load GitHub connection status', err)
      status.value = { state: 'disconnected', login: '', name: '', avatarUrl: '', message: '' }
    }
  }

  async function startDeviceFlow() {
    if (busy.value) return
    actionError.value = null
    busy.value = true
    try {
      deviceFlow.value = await StartDeviceFlow()
      card.value = 'device'
    } catch (err) {
      actionError.value = messageOf(err, 'Could not reach GitHub to start sign-in.')
    } finally {
      busy.value = false
    }
  }

  async function cancelDeviceFlow() {
    deviceFlow.value = null
    card.value = 'idle'
    try {
      await CancelDeviceFlow()
    } catch (err) {
      console.warn('Unable to cancel device flow', err)
    }
  }

  function useTokenInstead() {
    actionError.value = null
    card.value = 'token'
    void cancelPendingFlow()
  }

  async function cancelPendingFlow() {
    if (!deviceFlow.value) return
    deviceFlow.value = null
    try {
      await CancelDeviceFlow()
    } catch (err) {
      console.warn('Unable to cancel device flow', err)
    }
  }

  function backToStart() {
    actionError.value = null
    card.value = 'idle'
  }

  async function submitToken(token: string) {
    if (busy.value) return
    actionError.value = null
    busy.value = true
    try {
      status.value = await SetToken(token)
    } catch (err) {
      actionError.value = messageOf(err, 'GitHub rejected the token.')
    } finally {
      busy.value = false
    }
  }

  async function disconnect() {
    try {
      await Disconnect()
      card.value = 'idle'
      deviceFlow.value = null
      await reload()
    } catch (err) {
      console.warn('Unable to disconnect GitHub', err)
    }
  }

  onMounted(async () => {
    // connection:updated is a wake-up signal: the device-flow grant lands in a
    // Go goroutine, so state changes arrive here rather than as call results.
    // It names the provider, so another connector's change is ignored.
    useWailsEvent('connection:updated', (ev) => {
      if (ev.data !== PROVIDER) return
      void reload()
    })
    await reload()
  })

  return {
    status,
    connected,
    deviceFlow,
    card,
    error,
    busy,
    startDeviceFlow,
    cancelDeviceFlow,
    useTokenInstead,
    backToStart,
    submitToken,
    disconnect,
    reload,
  }
}

function messageOf(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message
  if (typeof err === 'string' && err) return err
  return fallback
}
