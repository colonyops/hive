import { describe, expect, it, beforeEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { useAuth } from '../useAuth'
import type { AuthStatus } from '../../types/auth'

const mocks = vi.hoisted(() => ({
  Status: vi.fn(),
  StartDeviceFlow: vi.fn(),
  CancelDeviceFlow: vi.fn(),
  SetToken: vi.fn(),
  SignOut: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/internal/desktop/auth/service', () => ({
  Status: mocks.Status,
  StartDeviceFlow: mocks.StartDeviceFlow,
  CancelDeviceFlow: mocks.CancelDeviceFlow,
  SetToken: mocks.SetToken,
  SignOut: mocks.SignOut,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: mocks.On,
  },
}))

function authStatus(state: string, login = ''): AuthStatus {
  return { state, login, name: '', avatarUrl: '', message: '' }
}

function withAuth() {
  let auth!: ReturnType<typeof useAuth>
  const wrapper = mount(defineComponent({
    setup() {
      auth = useAuth()
      return () => null
    },
  }))
  return { auth, wrapper }
}

describe('useAuth', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.Status.mockResolvedValue(authStatus('unauthenticated'))
  })

  it('loads status on mount and exposes authenticated', async () => {
    mocks.Status.mockResolvedValue(authStatus('authenticated', 'hayden'))
    const { auth } = withAuth()
    await flushPromises()
    expect(auth.status.value?.login).toBe('hayden')
    expect(auth.authenticated.value).toBe(true)
  })

  it('falls back to unauthenticated when status fails', async () => {
    mocks.Status.mockRejectedValue(new Error('boom'))
    const { auth } = withAuth()
    await flushPromises()
    expect(auth.status.value?.state).toBe('unauthenticated')
    expect(auth.authenticated.value).toBe(false)
  })

  it('starts the device flow and switches to the device card', async () => {
    mocks.StartDeviceFlow.mockResolvedValue({ userCode: 'AAAA-BBBB', verificationUri: 'https://github.com/login/device' })
    const { auth } = withAuth()
    await flushPromises()

    await auth.startDeviceFlow()
    expect(auth.card.value).toBe('device')
    expect(auth.deviceFlow.value?.userCode).toBe('AAAA-BBBB')
    expect(auth.error.value).toBeNull()
  })

  it('surfaces device flow start failures', async () => {
    mocks.StartDeviceFlow.mockRejectedValue(new Error('github: unreachable'))
    const { auth } = withAuth()
    await flushPromises()

    await auth.startDeviceFlow()
    expect(auth.card.value).toBe('idle')
    expect(auth.error.value).toContain('unreachable')
  })

  it('adopts the status returned by SetToken', async () => {
    mocks.SetToken.mockResolvedValue(authStatus('authenticated', 'hayden'))
    const { auth } = withAuth()
    await flushPromises()

    await auth.submitToken('pat-1')
    expect(mocks.SetToken).toHaveBeenCalledWith('pat-1')
    expect(auth.authenticated.value).toBe(true)
  })

  it('keeps the token card and reports rejection errors', async () => {
    mocks.SetToken.mockRejectedValue(new Error('GitHub rejected the token'))
    const { auth } = withAuth()
    await flushPromises()

    auth.useTokenInstead()
    await auth.submitToken('bad')
    expect(auth.card.value).toBe('token')
    expect(auth.error.value).toContain('rejected')
    expect(auth.authenticated.value).toBe(false)
  })

  it('cancels a pending device flow when switching to the token card', async () => {
    mocks.StartDeviceFlow.mockResolvedValue({ userCode: 'AAAA-BBBB', verificationUri: 'https://github.com/login/device' })
    mocks.CancelDeviceFlow.mockResolvedValue(undefined)
    const { auth } = withAuth()
    await flushPromises()

    await auth.startDeviceFlow()
    auth.useTokenInstead()
    await flushPromises()
    expect(auth.card.value).toBe('token')
    expect(auth.deviceFlow.value).toBeNull()
    expect(mocks.CancelDeviceFlow).toHaveBeenCalled()
  })

  it('reloads status when auth:updated fires', async () => {
    let handler: (() => void) | undefined
    mocks.On.mockImplementation((event: string, cb: () => void) => {
      if (event === 'auth:updated') handler = cb
      return () => {}
    })
    const { auth } = withAuth()
    await flushPromises()
    expect(auth.authenticated.value).toBe(false)

    mocks.Status.mockResolvedValue(authStatus('authenticated', 'hayden'))
    handler?.()
    await flushPromises()
    expect(auth.authenticated.value).toBe(true)
  })

  it('unsubscribes from auth:updated on unmount', async () => {
    const unsubscribe = vi.fn()
    mocks.On.mockReturnValue(unsubscribe)
    const { wrapper } = withAuth()
    await flushPromises()

    wrapper.unmount()
    expect(unsubscribe).toHaveBeenCalled()
  })

  it('returns to the start card when the backend pushes a device-flow failure', async () => {
    let handler: (() => void) | undefined
    mocks.On.mockImplementation((event: string, cb: () => void) => {
      if (event === 'auth:updated') handler = cb
      return () => {}
    })
    mocks.StartDeviceFlow.mockResolvedValue({ userCode: 'AAAA-BBBB', verificationUri: 'https://github.com/login/device' })
    const { auth } = withAuth()
    await flushPromises()

    await auth.startDeviceFlow()
    expect(auth.card.value).toBe('device')

    mocks.Status.mockResolvedValue({ state: 'unauthenticated', message: 'github: device flow: authorization denied', login: '', name: '', avatarUrl: '' })
    handler?.()
    await flushPromises()

    expect(auth.card.value).toBe('idle')
    expect(auth.deviceFlow.value).toBeNull()
    expect(auth.error.value).toContain('authorization denied')
  })

  it('shows the stored-token failure message on the idle card', async () => {
    mocks.Status.mockResolvedValue({ state: 'unauthenticated', message: 'Stored GitHub token is no longer valid.', login: '', name: '', avatarUrl: '' })
    const { auth } = withAuth()
    await flushPromises()

    expect(auth.error.value).toBe('Stored GitHub token is no longer valid.')
  })

  it('ignores a second submit while busy', async () => {
    mocks.SetToken.mockReturnValue(new Promise(() => {}))
    const { auth } = withAuth()
    await flushPromises()

    void auth.submitToken('pat-1')
    void auth.submitToken('pat-1')

    expect(mocks.SetToken).toHaveBeenCalledTimes(1)
  })
})
