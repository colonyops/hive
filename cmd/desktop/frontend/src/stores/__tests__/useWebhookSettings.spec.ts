import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useWebhookSettings } from '../useWebhookSettings'

const mocks = vi.hoisted(() => ({
  Settings: vi.fn(),
  SetSettings: vi.fn(),
  GeneratePort: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/webhookservice', () => mocks)

function stored(overrides: Record<string, unknown> = {}) {
  return {
    enabled: true,
    port: 24831,
    host: '127.0.0.1',
    baseUrl: 'http://127.0.0.1:24831/hooks/',
    running: true,
    restartRequired: false,
    startError: '',
    portOverridden: false,
    portMin: 20000,
    portMax: 32767,
    ...overrides,
  }
}

describe('useWebhookSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.Settings.mockResolvedValue(stored())
    mocks.SetSettings.mockResolvedValue(undefined)
    mocks.GeneratePort.mockResolvedValue(25000)
  })

  it('loads the listener settings on first use', async () => {
    const webhook = useWebhookSettings()
    expect(webhook.settings.value).toBeNull()

    await flushPromises()

    expect(webhook.settings.value?.port).toBe(24831)
    expect(webhook.error.value).toBeNull()
  })

  it('saves the draft over the stored settings and re-reads the result', async () => {
    const webhook = useWebhookSettings()
    await flushPromises()
    mocks.Settings.mockResolvedValue(stored({ enabled: false, port: 3000, restartRequired: true }))

    expect(await webhook.save({ enabled: false, port: 3000 })).toBe(true)

    expect(mocks.SetSettings).toHaveBeenCalledWith(stored({ enabled: false, port: 3000 }))
    expect(webhook.settings.value?.restartRequired).toBe(true)
  })

  it('reports a failed save and keeps the stored settings', async () => {
    mocks.SetSettings.mockRejectedValue(new Error('port in use'))
    const webhook = useWebhookSettings()
    await flushPromises()

    expect(await webhook.save({ enabled: true, port: 80 })).toBe(false)

    expect(webhook.error.value).toBe('port in use')
    expect(webhook.settings.value?.port).toBe(24831)
    await webhook.reload()
    expect(webhook.error.value).toBeNull()
  })

  it('does not save before the settings have loaded', async () => {
    mocks.Settings.mockReturnValue(new Promise(() => {}))
    const webhook = useWebhookSettings()

    expect(await webhook.save({ enabled: true, port: 1 })).toBe(false)
    expect(mocks.SetSettings).not.toHaveBeenCalled()
  })

  it('generates a port and reports a failure to do so', async () => {
    const webhook = useWebhookSettings()
    await flushPromises()

    expect(await webhook.generatePort()).toBe(25000)

    mocks.GeneratePort.mockRejectedValue(new Error('no free port'))
    expect(await webhook.generatePort()).toBeUndefined()
    expect(webhook.error.value).toBe('no free port')
  })
})
