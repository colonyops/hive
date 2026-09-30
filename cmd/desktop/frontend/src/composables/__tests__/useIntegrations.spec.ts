import { describe, expect, it, beforeEach, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { isConnected, takesCredential, useIntegrations } from '../useIntegrations'
import type { Integration } from '../../types/integrations'

const mocks = vi.hoisted(() => ({
  List: vi.fn(),
  On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/integrationsservice', () => ({
  List: mocks.List,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: { On: mocks.On },
}))

function integration(overrides: Partial<Integration> = {}): Integration {
  return {
    key: 'github',
    title: 'GitHub source',
    stability: 'stable',
    provider: 'github',
    types: ['sources.github'],
    accounts: [],
    envOverride: false,
    ...overrides,
  }
}

function withIntegrations() {
  let api!: ReturnType<typeof useIntegrations>
  const wrapper = mount(defineComponent({
    setup() {
      api = useIntegrations()
      return () => null
    },
  }))
  return { api, wrapper }
}

describe('useIntegrations', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.List.mockResolvedValue([integration()])
  })

  it('loads the registry projection on mount', async () => {
    mocks.List.mockResolvedValue([integration({ accounts: ['octocat'] })])
    const { api } = withIntegrations()
    await flushPromises()

    expect(api.loaded.value).toBe(true)
    expect(api.integrations.value).toHaveLength(1)
    expect(api.accountsFor('github')).toEqual(['octocat'])
  })

  // The generator types every Go slice as nullable. Normalizing at this seam
  // is what lets the screen and the node editor read accounts without a guard.
  it('normalizes a null account list to an empty one', async () => {
    mocks.List.mockResolvedValue([{ ...integration(), accounts: null }])
    const { api } = withIntegrations()
    await flushPromises()

    expect(api.integrations.value[0].accounts).toEqual([])
    expect(api.accountsFor('github')).toEqual([])
  })

  it('survives a null list', async () => {
    mocks.List.mockResolvedValue(null)
    const { api } = withIntegrations()
    await flushPromises()

    expect(api.integrations.value).toEqual([])
    expect(api.loaded.value).toBe(true)
  })

  it('reports a load failure without leaving the screen unloaded', async () => {
    mocks.List.mockRejectedValue(new Error('boom'))
    const { api } = withIntegrations()
    await flushPromises()

    expect(api.error.value).toContain('boom')
    expect(api.loaded.value).toBe(true)
  })

  it('builds credential refs in the form a node config takes', async () => {
    mocks.List.mockResolvedValue([integration({ accounts: ['octocat', 'hubot'] })])
    const { api } = withIntegrations()
    await flushPromises()

    expect(api.credentialRefsFor('github')).toEqual(['github/octocat', 'github/hubot'])
  })

  // Unlike useGitHubConnection this spans providers, so it must not filter:
  // any provider connecting or disconnecting changes what this list reports.
  it('reloads on connection:updated for any provider', async () => {
    let handler: ((ev: { data: unknown }) => void) | undefined
    mocks.On.mockImplementation((event: string, cb: (ev: { data: unknown }) => void) => {
      if (event === 'connection:updated') handler = cb
      return () => {}
    })
    const { api } = withIntegrations()
    await flushPromises()
    expect(api.accountsFor('github')).toEqual([])

    mocks.List.mockResolvedValue([integration({ accounts: ['octocat'] })])
    handler?.({ data: 'grafana' })
    await flushPromises()

    expect(api.accountsFor('github')).toEqual(['octocat'])
  })

  it('unsubscribes on unmount', async () => {
    const unsubscribe = vi.fn()
    mocks.On.mockReturnValue(unsubscribe)
    const { wrapper } = withIntegrations()
    await flushPromises()

    wrapper.unmount()
    expect(unsubscribe).toHaveBeenCalled()
  })
})

describe('isConnected', () => {
  it('is true with a stored account', () => {
    expect(isConnected(integration({ accounts: ['octocat'] }))).toBe(true)
  })

  // The override authenticates every fetch while naming no account, so an
  // empty list is not the same as disconnected.
  it('is true with only an environment override', () => {
    expect(isConnected(integration({ accounts: [], envOverride: true }))).toBe(true)
  })

  it('is false with neither', () => {
    expect(isConnected(integration())).toBe(false)
  })
})

describe('takesCredential', () => {
  it('is false for a connector with no provider', () => {
    expect(takesCredential(integration({ key: 'sources.webhook', provider: '' }))).toBe(false)
  })

  it('is true for one with a provider', () => {
    expect(takesCredential(integration())).toBe(true)
  })
})
