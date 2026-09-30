import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AppearanceSettingsView from '../AppearanceSettingsView.vue'
import { resetInstalledFontsForTests } from '../../composables/useInstalledFonts'

const mocks = vi.hoisted(() => ({
  AppearanceSettings: vi.fn(),
  Fonts: vi.fn(),
  SetFontFamily: vi.fn(),
  SetMonoFontFamily: vi.fn(),
  SetTheme: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice', () => ({
  AppearanceSettings: mocks.AppearanceSettings,
  Fonts: mocks.Fonts,
  SetFontFamily: mocks.SetFontFamily,
  SetMonoFontFamily: mocks.SetMonoFontFamily,
  SetTheme: mocks.SetTheme,
}))

beforeEach(() => {
  vi.clearAllMocks()
  resetInstalledFontsForTests()
  mocks.AppearanceSettings.mockResolvedValue({ theme: 'dark', fontFamily: '', monoFontFamily: '' })
  mocks.Fonts.mockResolvedValue({
    all: ['Fira Code', 'Helvetica Neue', 'JetBrains Mono'],
    monospace: ['Fira Code', 'JetBrains Mono'],
  })
  mocks.SetFontFamily.mockResolvedValue(undefined)
  mocks.SetMonoFontFamily.mockResolvedValue(undefined)
  mocks.SetTheme.mockResolvedValue(undefined)
  document.body.innerHTML = ''
})

async function optionLabels(testid: string): Promise<string[]> {
  const wrapper = mount(AppearanceSettingsView, { attachTo: document.body })
  await flushPromises()
  await wrapper.get(`[data-testid="${testid}"]`).trigger('click')
  await flushPromises()
  return Array.from(document.querySelectorAll('[role="option"]')).map((el) => el.textContent?.trim() ?? '')
}

describe('AppearanceSettingsView', () => {
  it('offers the bundled face and the platform stack ahead of installed families', async () => {
    expect(await optionLabels('settings-appearance-font-family-select')).toEqual([
      'Inter · bundled',
      'System',
      'Fira Code',
      'Helvetica Neue',
      'JetBrains Mono',
    ])
  })

  // The mono picker is limited to fixed-pitch families, and the bundled one is
  // withheld from the installed list so choosing it cannot pin today's face by
  // name — the empty selection is what tracks the shipped default.
  it('limits the monospace picker to fixed-pitch families, minus the bundled one', async () => {
    expect(await optionLabels('settings-appearance-mono-font-family-select')).toEqual([
      'JetBrains Mono · bundled',
      'System',
      'Fira Code',
    ])
  })

  it('persists a chosen interface font without touching the monospace one', async () => {
    const wrapper = mount(AppearanceSettingsView, { attachTo: document.body })
    await flushPromises()

    await wrapper.get('[data-testid="settings-appearance-font-family-select"]').trigger('click')
    await flushPromises()
    const option = Array.from(document.querySelectorAll<HTMLElement>('[role="option"] button'))
      .find((el) => el.textContent?.trim() === 'Helvetica Neue')!
    option.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()

    expect(mocks.SetFontFamily).toHaveBeenCalledWith('Helvetica Neue')
    expect(mocks.SetMonoFontFamily).not.toHaveBeenCalled()
  })

  it('scans for installed fonts only once the pane is on screen', async () => {
    expect(mocks.Fonts).not.toHaveBeenCalled()

    mount(AppearanceSettingsView, { attachTo: document.body })
    await flushPromises()

    expect(mocks.Fonts).toHaveBeenCalledTimes(1)
  })

  // A machine with no readable font directory still has to offer the shipped
  // faces and the platform stack.
  it('keeps the bundled and system options when the scan fails', async () => {
    mocks.Fonts.mockRejectedValue(new Error('no font directory'))

    expect(await optionLabels('settings-appearance-font-family-select')).toEqual([
      'Inter · bundled',
      'System',
    ])
  })
})
