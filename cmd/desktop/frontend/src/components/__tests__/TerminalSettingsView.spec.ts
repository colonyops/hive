import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import TerminalSettingsView from '../TerminalSettingsView.vue'
import {
  defaultTerminalFontSizePx,
  defaultTerminalFontWeight,
  defaultTerminalFontWeightBold,
  setTerminalFontSize,
  setTerminalFontWeight,
  setTerminalFontWeightBold,
} from '../../composables/useTerminalFont'
import { TERMINAL_FONT } from '../../lib/terminalFaces'
import { setTerminalShowWindows } from '../../composables/useTerminalShowWindows'
import { resetInstalledFontsForTests } from '../../composables/useInstalledFonts'

const mocks = vi.hoisted(() => ({
  SetTerminalShowWindows: vi.fn(),
  SetTerminalPoolSize: vi.fn(),
  SetTerminalFontFamily: vi.fn(),
  SetTerminalFontWeights: vi.fn(),
  Fonts: vi.fn().mockResolvedValue({ all: ['Fira Code', 'Menlo'], monospace: ['Fira Code', 'Menlo'] }),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: vi.fn().mockResolvedValue({
    theme: '',
    terminalFontSizePx: 13,
    terminalFontFamily: '',
    terminalFontWeight: 0,
    terminalFontWeightBold: 0,
    terminalShowWindows: true,
    terminalPoolSize: 3,
  }),
  Fonts: mocks.Fonts,
  SetTheme: vi.fn(),
  SetTerminalFontSize: vi.fn(),
  SetTerminalFontFamily: mocks.SetTerminalFontFamily,
  SetTerminalFontWeights: mocks.SetTerminalFontWeights,
  SetTerminalShowWindows: mocks.SetTerminalShowWindows,
  SetTerminalPoolSize: mocks.SetTerminalPoolSize,
}))

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  // The scan is a module singleton that runs once, so without this the second
  // mount in this file would keep the first test's list.
  resetInstalledFontsForTests()
  mocks.Fonts.mockResolvedValue({ all: ['Fira Code', 'Menlo'], monospace: ['Fira Code', 'Menlo'] })
  document.body.innerHTML = ''
})

describe('TerminalSettingsView', () => {
  it('nudges the terminal font size by the ladder step', async () => {
    const wrapper = mount(TerminalSettingsView)
    const value = (): string => wrapper.find('[data-testid="settings-terminal-font-size-value"]').text()

    expect(value()).toBe(`${defaultTerminalFontSizePx}px`)

    await wrapper.find('[data-testid="settings-terminal-font-size-increase"]').trigger('click')
    expect(value()).toBe(`${defaultTerminalFontSizePx + 2}px`)

    await wrapper.find('[data-testid="settings-terminal-font-size-decrease"]').trigger('click')
    expect(value()).toBe(`${defaultTerminalFontSizePx}px`)

    // The size is a module singleton; put the default back for later tests.
    setTerminalFontSize(defaultTerminalFontSizePx)
  })

  // #181: the terminal shipped with no weight control at all, so normal cells
  // rendered at whatever the atlas drew.
  it('reflects and changes the terminal font weight', async () => {
    const wrapper = mount(TerminalSettingsView)

    expect(
      wrapper
        .find(`[data-testid="settings-terminal-font-weight-${defaultTerminalFontWeight}"]`)
        .attributes('aria-selected'),
    ).toBe('true')

    await wrapper.find('[data-testid="settings-terminal-font-weight-400"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="settings-terminal-font-weight-400"]').attributes('aria-selected')).toBe('true')
    expect(mocks.SetTerminalFontWeights).toHaveBeenCalledWith(400, defaultTerminalFontWeightBold)

    setTerminalFontWeight(defaultTerminalFontWeight)
  })

  // Both weights go through one setter: written separately, a caller could land
  // a normal weight above the bold one.
  it('persists both weights when only the bold one changes', async () => {
    const wrapper = mount(TerminalSettingsView)
    mocks.SetTerminalFontWeights.mockClear()

    await wrapper.find('[data-testid="settings-terminal-font-weight-bold-600"]').trigger('click')
    await flushPromises()

    expect(mocks.SetTerminalFontWeights).toHaveBeenCalledWith(defaultTerminalFontWeight, 600)

    setTerminalFontWeightBold(defaultTerminalFontWeightBold)
  })

  // The scan is what the webview cannot do for itself, and the bundled face
  // leads the list whether or not it is also installed system-wide.
  it('offers the installed monospace families with the bundled face first', async () => {
    const wrapper = mount(TerminalSettingsView)
    await flushPromises()

    await wrapper.get('[data-testid="settings-terminal-font-family-select"]').trigger('click')
    await flushPromises()

    // The families can only have come from the binding — the scan is what the
    // webview cannot do for itself, and its result is cached for the process.
    const labels = Array.from(document.querySelectorAll('[role="option"]'))
    expect(labels.map((el) => el.textContent?.trim())).toEqual([`${TERMINAL_FONT} · bundled`, 'Fira Code', 'Menlo'])
    wrapper.unmount()
  })

  it('reflects and toggles the terminal window listing', async () => {
    const wrapper = mount(TerminalSettingsView)

    const toggle = wrapper.get('[data-testid="settings-terminal-show-windows"]')
    expect(toggle.attributes('aria-checked')).toBe('true')

    await toggle.trigger('click')
    await flushPromises()

    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(mocks.SetTerminalShowWindows).toHaveBeenCalledWith(false)

    // The setting is a module singleton; put the default back for later tests.
    setTerminalShowWindows(true)
  })

  it('reflects and changes the terminal warm-session count', async () => {
    const wrapper = mount(TerminalSettingsView)

    expect(wrapper.find('[data-testid="settings-terminal-pool-size-3"]').attributes('aria-selected')).toBe('true')

    await wrapper.find('[data-testid="settings-terminal-pool-size-5"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="settings-terminal-pool-size-5"]').attributes('aria-selected')).toBe('true')
    expect(mocks.SetTerminalPoolSize).toHaveBeenCalledWith(5)
  })
})
