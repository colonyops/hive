import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ObservabilitySettingsView from '../ObservabilitySettingsView.vue'

const mocks = vi.hoisted(() => ({
  Settings: vi.fn(),
  OpenURL: vi.fn(),
}))

vi.mock(
  '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/observabilityservice',
  () => ({
    Settings: mocks.Settings,
  }),
)

vi.mock('@wailsio/runtime', () => ({ Browser: { OpenURL: mocks.OpenURL } }))

function view(overrides: Record<string, unknown> = {}) {
  return {
    otlp: { enabled: false, configured: false, running: false, restartRequired: false },
    profiles: { enabled: false, configured: false, running: false, restartRequired: false },
    startError: '',
    ...overrides,
  }
}

function mountView() {
  return mount(ObservabilitySettingsView, {
    global: { stubs: { RuntimeDashboard: { template: '<div data-testid="runtime-dashboard" />' } } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Settings.mockResolvedValue(view())
})

describe('ObservabilitySettingsView', () => {
  it('shows enabled, configured, and running status for each exporter', async () => {
    mocks.Settings.mockResolvedValue(
      view({
        otlp: { enabled: true, configured: true, running: true, restartRequired: false },
        profiles: { enabled: false, configured: true, running: false, restartRequired: false },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="observability-otlp-status"]').text()).toBe('Exporting')
    expect(wrapper.get('[data-testid="observability-otlp-enabled"]').text()).toBe('Enabled')
    expect(wrapper.get('[data-testid="observability-otlp-configured"]').text()).toBe('Configured')
    expect(wrapper.get('[data-testid="observability-profiles-status"]').text()).toBe('Ready')
    expect(wrapper.get('[data-testid="observability-profiles-enabled"]').text()).toBe('Disabled')
    expect(wrapper.get('[data-testid="observability-profiles-configured"]').text()).toBe('Configured')
  })

  it('marks a settings.yaml change that needs a restart', async () => {
    mocks.Settings.mockResolvedValue(
      view({
        otlp: { enabled: true, configured: true, running: false, restartRequired: true },
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="observability-restart-banner"]').text()).toContain('Restart Hive')
    expect(wrapper.get('[data-testid="observability-otlp-status"]').text()).toBe('Restart needed')
  })

  it('shows exporter startup errors without claiming the backend is unhealthy', async () => {
    mocks.Settings.mockResolvedValue(
      view({
        otlp: { enabled: true, configured: true, running: false, restartRequired: false },
        startError: 'token reference could not be resolved',
      }),
    )

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="observability-otlp-status"]').text()).toBe('Error')
    expect(wrapper.get('[data-testid="observability-start-error"]').text()).toContain(
      'token reference could not be resolved',
    )
  })

  it('opens the telemetry configuration guide', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-testid="observability-docs"]').trigger('click')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://hivedesktop.com/configuration/settings/#telemetry')
  })
})
