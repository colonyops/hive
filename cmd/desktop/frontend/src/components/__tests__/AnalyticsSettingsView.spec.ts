import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import AnalyticsSettingsView from '../AnalyticsSettingsView.vue'
import { resetErrorDialogForTests, useErrorDialog } from '../../composables/useErrorDialog'

const mocks = vi.hoisted(() => ({ Summary: vi.fn(), SetEnabled: vi.fn(), Clear: vi.fn() }))
vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/analyticsservice',
  () => mocks,
)

function summary(overrides = {}) {
  return {
    counts: { cliCommands: 0, hiveSessions: 0, terminalStarts: 0 },
    enabled: true,
    effectiveEnabled: true,
    active: true,
    restartNeeded: false,
    environmentOverride: false,
    retentionDays: 90,
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  resetErrorDialogForTests()
  mocks.Summary.mockResolvedValue(summary())
  mocks.SetEnabled.mockResolvedValue(undefined)
  mocks.Clear.mockResolvedValue(undefined)
})
afterEach(() => {
  document.body.innerHTML = ''
})

describe('AnalyticsSettingsView', () => {
  it('shows empty history and the three flushed counts after refresh', async () => {
    const wrapper = mount(AnalyticsSettingsView)
    await flushPromises()
    expect(wrapper.get('[data-testid="analytics-cli-count"]').text()).toBe('0')
    mocks.Summary.mockResolvedValue(summary({ counts: { cliCommands: 7, hiveSessions: 3, terminalStarts: 2 } }))
    await wrapper.get('[data-testid="analytics-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="analytics-cli-count"]').text()).toBe('7')
    expect(wrapper.get('[data-testid="analytics-session-count"]').text()).toBe('3')
    expect(wrapper.get('[data-testid="analytics-terminal-count"]').text()).toBe('2')
    expect(wrapper.text()).toContain('including failures')
    wrapper.unmount()
  })

  it('saves collection changes and labels restart and environment override', async () => {
    const wrapper = mount(AnalyticsSettingsView)
    await flushPromises()
    mocks.Summary.mockResolvedValue(summary({ enabled: false, restartNeeded: true, environmentOverride: true }))
    await wrapper.get('[data-testid="analytics-enabled"]').trigger('click')
    await flushPromises()
    expect(mocks.SetEnabled).toHaveBeenCalledWith(false)
    expect(wrapper.get('[data-testid="analytics-restart"]').text()).toContain('Restart Hive Desktop')
    expect(wrapper.get('[data-testid="analytics-environment"]').text()).toContain('HIVE_ANALYTICS_ENABLED')
    wrapper.unmount()
  })

  it('does not clear until confirmed and reloads the history afterwards', async () => {
    const wrapper = mount(AnalyticsSettingsView, { attachTo: document.body })
    await flushPromises()
    await wrapper.get('[data-testid="analytics-clear"]').trigger('click')
    await flushPromises()
    expect(mocks.Clear).not.toHaveBeenCalled()
    const dialog = document.querySelector('[data-testid="analytics-clear-confirmation-confirm"]') as HTMLElement
    expect(dialog).not.toBeNull()
    dialog.click()
    await flushPromises()
    expect(mocks.Clear).toHaveBeenCalledOnce()
    expect(mocks.Summary).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('reports load and save failures without changing the saved switch', async () => {
    mocks.Summary.mockRejectedValueOnce(new Error('database locked'))
    const wrapper = mount(AnalyticsSettingsView)
    await flushPromises()
    expect(wrapper.get('[data-testid="analytics-error"]').text()).toContain('database locked')
    await wrapper.get('[data-testid="analytics-refresh"]').trigger('click')
    await flushPromises()
    mocks.SetEnabled.mockRejectedValueOnce(new Error('config is read only'))
    await wrapper.get('[data-testid="analytics-enabled"]').trigger('click')
    await flushPromises()
    expect(useErrorDialog().current.value?.detail).toContain('config is read only')
    expect(wrapper.get('[data-testid="analytics-enabled"]').attributes('aria-checked')).toBe('true')
    wrapper.unmount()
  })
})
