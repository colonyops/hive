import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Events } from '@wailsio/runtime'
import { SetToken, SignOut, StartDeviceFlow, Status } from '../../bindings/github.com/colonyops/hive/internal/desktop/auth/service'
import { CancelDeviceFlow } from '../../bindings/github.com/colonyops/hive/internal/desktop/auth/service'
import type { AuthStatus, DeviceFlowInfo } from '../types/auth'

export type OnboardingCard = 'idle' | 'device' | 'token'

export function useAuth() {
  // null until the first Status() resolves, so the app can hold a loading
  // frame instead of flashing onboarding at an authenticated user.
  const status = ref<AuthStatus | null>(null)
  const deviceFlow = ref<DeviceFlowInfo | null>(null)
  const card = ref<OnboardingCard>('idle')
  // Errors from the active card's action; cleared on card switches.
  const actionError = ref<string | null>(null)
  // Backend-pushed failures ride Status.Message (auth:updated carries no
  // payload); show them on the idle card unless a local action error is fresher.
  const error = computed(() => {
    if (actionError.value) return actionError.value
    const current = status.value
    if (card.value === 'idle' && current && current.state !== 'authenticated' && current.message) return current.message
    return null
  })
  const busy = ref(false)
  let unsubscribe: (() => void) | undefined

  const authenticated = computed(() => status.value?.state === 'authenticated')

  async function reload() {
    try {
      const next = await Status()
      status.value = next
      // A failure push while the device card waits means the flow died
      // (denied, expired, validation failed): fall back to the start card so
      // a dead user code is not left on screen.
      if (card.value === 'device' && next.state !== 'authenticated' && next.message) {
        card.value = 'idle'
        deviceFlow.value = null
      }
    } catch (err) {
      console.warn('Unable to load auth status', err)
      status.value = { state: 'unauthenticated', login: '', name: '', avatarUrl: '', message: '' }
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

  async function signOut() {
    try {
      await SignOut()
      card.value = 'idle'
      deviceFlow.value = null
      await reload()
    } catch (err) {
      console.warn('Unable to sign out', err)
    }
  }

  onMounted(async () => {
    // auth:updated is a wake-up signal: the device-flow grant lands in a Go
    // goroutine, so state changes arrive here rather than as call results.
    unsubscribe = Events.On('auth:updated', () => { void reload() })
    await reload()
  })

  onUnmounted(() => {
    unsubscribe?.()
  })

  return {
    status,
    authenticated,
    deviceFlow,
    card,
    error,
    busy,
    startDeviceFlow,
    cancelDeviceFlow,
    useTokenInstead,
    backToStart,
    submitToken,
    signOut,
    reload,
  }
}

function messageOf(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message
  if (typeof err === 'string' && err) return err
  return fallback
}
