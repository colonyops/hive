import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import GithubIntegrationDrawer from '../GithubIntegrationDrawer.vue'

const mocks = vi.hoisted(() => ({
  GithubSettings: vi.fn(),
  SetGithubSettings: vi.fn(),
}))

vi.mock('../../../../bindings/github.com/colonyops/hive/desktop/settingsservice', () => mocks)

function mountDrawer() {
  return mount(GithubIntegrationDrawer, { global: { stubs: { Teleport: true } } })
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.GithubSettings.mockResolvedValue({ pollIntervalSeconds: 120, minPollIntervalSeconds: 60 })
  mocks.SetGithubSettings.mockResolvedValue(undefined)
})

describe('GithubIntegrationDrawer', () => {
  it('loads the current GitHub interval', async () => {
    const wrapper = mountDrawer()
    await flushPromises()

    expect(mocks.GithubSettings).toHaveBeenCalledOnce()
    expect((wrapper.get('[data-testid="github-poll-interval-input"]').element as HTMLInputElement).value).toBe('120')
  })

  it('disables Save below the server-provided minimum and shows the contract hint', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('[data-testid="github-poll-interval-input"]').setValue('30')

    expect(wrapper.get('[data-testid="github-poll-interval-hint"]').text()).toBe("Minimum 60s — GitHub's polling contract")
    expect(wrapper.get('[data-testid="github-settings-save"]').attributes('disabled')).toBeDefined()
  })

  it('saves a valid interval and closes', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    await wrapper.get('[data-testid="github-poll-interval-input"]').setValue('180')
    await wrapper.get('[data-testid="github-settings-save"]').trigger('click')
    await flushPromises()

    expect(mocks.SetGithubSettings).toHaveBeenCalledWith({ pollIntervalSeconds: 180, minPollIntervalSeconds: 60 })
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('closes from Escape or the backdrop without saving', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.get('[data-testid="github-integration-backdrop"]').trigger('click')

    expect(wrapper.emitted('close')).toHaveLength(2)
    expect(mocks.SetGithubSettings).not.toHaveBeenCalled()
  })
})
