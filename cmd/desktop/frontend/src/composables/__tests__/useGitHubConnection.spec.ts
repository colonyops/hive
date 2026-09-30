import { describe, expect, it, beforeEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { useGitHubConnection } from '../useGitHubConnection'
import type { ConnectionStatus } from '../../types/github'

const mocks = vi.hoisted(() => ({
  Status: vi.fn(),
  StartDeviceFlow: vi.fn(),
  CancelDeviceFlow: vi.fn(),
  SetToken: vi.fn(),
  Disconnect: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/githubservice', () => ({
  Status: mocks.Status,
  StartDeviceFlow: mocks.StartDeviceFlow,
  CancelDeviceFlow: mocks.CancelDeviceFlow,
  SetToken: mocks.SetToken,
  Disconnect: mocks.Disconnect,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: mocks.On,
  },
}))

function connectionStatus(state: string, login = ''): ConnectionStatus {
  return { state, login, name: '', avatarUrl: '', message: '' }
}

function withConnection() {
  let github!: ReturnType<typeof useGitHubConnection>
  const wrapper = mount(defineComponent({
    setup() {
      github = useGitHubConnection()
      return () => null
    },
  }))
  return { github, wrapper }
}

describe('useGitHubConnection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.Status.mockResolvedValue(connectionStatus('disconnected'))
  })

  it('loads status on mount and exposes connected', async () => {
    mocks.Status.mockResolvedValue(connectionStatus('connected', 'octocat'))
    const { github } = withConnection()
    await flushPromises()
    expect(github.status.value?.login).toBe('octocat')
    expect(github.connected.value).toBe(true)
  })

  it('falls back to disconnected when status fails', async () => {
    mocks.Status.mockRejectedValue(new Error('boom'))
    const { github } = withConnection()
    await flushPromises()
    expect(github.status.value?.state).toBe('disconnected')
    expect(github.connected.value).toBe(false)
  })

  it('starts the device flow and switches to the device card', async () => {
    mocks.StartDeviceFlow.mockResolvedValue({ userCode: 'AAAA-BBBB', verificationUri: 'https://github.com/login/device' })
    const { github } = withConnection()
    await flushPromises()

    await github.startDeviceFlow()
    expect(github.card.value).toBe('device')
    expect(github.deviceFlow.value?.userCode).toBe('AAAA-BBBB')
    expect(github.error.value).toBeNull()
  })

  it('surfaces device flow start failures', async () => {
    mocks.StartDeviceFlow.mockRejectedValue(new Error('github: unreachable'))
    const { github } = withConnection()
    await flushPromises()

    await github.startDeviceFlow()
    expect(github.card.value).toBe('idle')
    expect(github.error.value).toContain('unreachable')
  })

  it('adopts the status returned by SetToken', async () => {
    mocks.SetToken.mockResolvedValue(connectionStatus('connected', 'octocat'))
    const { github } = withConnection()
    await flushPromises()

    await github.submitToken('pat-1')
    expect(mocks.SetToken).toHaveBeenCalledWith('pat-1')
    expect(github.connected.value).toBe(true)
  })

  it('keeps the token card and reports rejection errors', async () => {
    mocks.SetToken.mockRejectedValue(new Error('GitHub rejected the token'))
    const { github } = withConnection()
    await flushPromises()

    github.useTokenInstead()
    await github.submitToken('bad')
    expect(github.card.value).toBe('token')
    expect(github.error.value).toContain('rejected')
    expect(github.connected.value).toBe(false)
  })

  it('cancels a pending device flow when switching to the token card', async () => {
    mocks.StartDeviceFlow.mockResolvedValue({ userCode: 'AAAA-BBBB', verificationUri: 'https://github.com/login/device' })
    mocks.CancelDeviceFlow.mockResolvedValue(undefined)
    const { github } = withConnection()
    await flushPromises()

    await github.startDeviceFlow()
    github.useTokenInstead()
    await flushPromises()
    expect(github.card.value).toBe('token')
    expect(github.deviceFlow.value).toBeNull()
    expect(mocks.CancelDeviceFlow).toHaveBeenCalled()
  })

  it('reloads status when connection:updated names github', async () => {
    let handler: ((ev: { data: unknown }) => void) | undefined
    mocks.On.mockImplementation((event: string, cb: (ev: { data: unknown }) => void) => {
      if (event === 'connection:updated') handler = cb
      return () => {}
    })
    const { github } = withConnection()
    await flushPromises()
    expect(github.connected.value).toBe(false)

    mocks.Status.mockResolvedValue(connectionStatus('connected', 'octocat'))
    handler?.({ data: 'github' })
    await flushPromises()
    expect(github.connected.value).toBe(true)
  })

  // The event names the provider so a connector this composable does not own
  // cannot cost it a re-read. With several integrations connected, an
  // unfiltered wake-up would refetch GitHub's status on every one of them.
  it('ignores connection:updated for another provider', async () => {
    let handler: ((ev: { data: unknown }) => void) | undefined
    mocks.On.mockImplementation((event: string, cb: (ev: { data: unknown }) => void) => {
      if (event === 'connection:updated') handler = cb
      return () => {}
    })
    const { github } = withConnection()
    await flushPromises()
    expect(mocks.Status).toHaveBeenCalledTimes(1)

    handler?.({ data: 'grafana' })
    await flushPromises()
    expect(mocks.Status).toHaveBeenCalledTimes(1)
    expect(github.connected.value).toBe(false)
  })

  it('unsubscribes from connection:updated on unmount', async () => {
    const unsubscribe = vi.fn()
    mocks.On.mockReturnValue(unsubscribe)
    const { wrapper } = withConnection()
    await flushPromises()

    wrapper.unmount()
    expect(unsubscribe).toHaveBeenCalled()
  })

  it('returns to the start card when the backend pushes a device-flow failure', async () => {
    let handler: ((ev: { data: unknown }) => void) | undefined
    mocks.On.mockImplementation((event: string, cb: (ev: { data: unknown }) => void) => {
      if (event === 'connection:updated') handler = cb
      return () => {}
    })
    mocks.StartDeviceFlow.mockResolvedValue({ userCode: 'AAAA-BBBB', verificationUri: 'https://github.com/login/device' })
    const { github } = withConnection()
    await flushPromises()

    await github.startDeviceFlow()
    expect(github.card.value).toBe('device')

    mocks.Status.mockResolvedValue({ state: 'disconnected', message: 'github: device flow: authorization denied', login: '', name: '', avatarUrl: '' })
    handler?.({ data: 'github' })
    await flushPromises()

    expect(github.card.value).toBe('idle')
    expect(github.deviceFlow.value).toBeNull()
    expect(github.error.value).toContain('authorization denied')
  })

  it('shows the stored-token failure message on the idle card', async () => {
    mocks.Status.mockResolvedValue({ state: 'disconnected', message: 'Stored GitHub token is no longer valid.', login: '', name: '', avatarUrl: '' })
    const { github } = withConnection()
    await flushPromises()

    expect(github.error.value).toBe('Stored GitHub token is no longer valid.')
  })

  it('ignores a second submit while busy', async () => {
    mocks.SetToken.mockReturnValue(new Promise(() => {}))
    const { github } = withConnection()
    await flushPromises()

    void github.submitToken('pat-1')
    void github.submitToken('pat-1')

    expect(mocks.SetToken).toHaveBeenCalledTimes(1)
  })

  it('disconnects and re-reads status', async () => {
    mocks.Status.mockResolvedValue(connectionStatus('connected', 'octocat'))
    mocks.Disconnect.mockResolvedValue(undefined)
    const { github } = withConnection()
    await flushPromises()
    expect(github.connected.value).toBe(true)

    mocks.Status.mockResolvedValue(connectionStatus('disconnected'))
    await github.disconnect()
    expect(mocks.Disconnect).toHaveBeenCalled()
    expect(github.connected.value).toBe(false)
  })
})
