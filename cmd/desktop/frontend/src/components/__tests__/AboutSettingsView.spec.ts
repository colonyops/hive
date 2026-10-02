import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AboutSettingsView from '../AboutSettingsView.vue'

const mocks = vi.hoisted(() => ({
  Build: vi.fn(),
  Status: vi.fn(),
  SetEnabled: vi.fn(),
  CheckNow: vi.fn(),
  OpenURL: vi.fn(),
  SetText: vi.fn(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice', () => ({
  Build: mocks.Build,
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/updaterservice', () => ({
  Status: mocks.Status,
  SetEnabled: mocks.SetEnabled,
  CheckNow: mocks.CheckNow,
}))
vi.mock('@wailsio/runtime', () => ({
  Browser: { OpenURL: mocks.OpenURL },
  Clipboard: { SetText: mocks.SetText },
}))

function updateInfo(overrides: Record<string, unknown> = {}) {
  return {
    enabled: true,
    available: false,
    currentVersion: '1.4.0',
    latestVersion: '',
    notes: '',
    checkedAt: '',
    ...overrides,
  }
}

function buildInfo(overrides: Record<string, unknown> = {}) {
  return {
    version: '1.4.0',
    commit: 'abc1234',
    date: '2026-07-01T12:00:00Z',
    channel: 'stable',
    os: 'darwin',
    arch: 'arm64',
    goVersion: 'go1.24.2',
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Build.mockResolvedValue(buildInfo())
  mocks.Status.mockResolvedValue(updateInfo())
  mocks.SetEnabled.mockResolvedValue(undefined)
  mocks.CheckNow.mockResolvedValue(updateInfo())
  mocks.OpenURL.mockResolvedValue(undefined)
  mocks.SetText.mockResolvedValue(undefined)
})

describe('AboutSettingsView', () => {
  it('renders a card per build fact', async () => {
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="about-build-version"]').text()).toBe('1.4.0')
    expect(wrapper.find('[data-testid="about-build-commit"]').text()).toBe('abc1234')
    expect(wrapper.find('[data-testid="about-build-date"]').text()).toContain('2026')
    expect(wrapper.find('[data-testid="about-build-platform"]').text()).toBe('macOS · arm64')
    expect(wrapper.find('[data-testid="about-stat-platform"]').text()).toContain('go1.24.2')
  })

  // The source repository is private, so a commit, tag or release page is a
  // 404 for everyone but its author. Only the product site is linkable.
  it('offers no link into the source repository', async () => {
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    expect(wrapper.html()).not.toContain('github.com')

    await wrapper.find('[data-testid="about-link-docs"]').trigger('click')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://hivedesktop.com/getting-started/')

    await wrapper.find('[data-testid="about-link-updates"]').trigger('click')
    expect(mocks.OpenURL).toHaveBeenCalledWith('https://hivedesktop.com/configuration/settings/#updates')
  })

  it('reads a build with no published release as one that cannot self-update', async () => {
    mocks.Build.mockResolvedValue(buildInfo({ version: 'dev', commit: 'HEAD', channel: '' }))
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="about-build-version"]').text()).toBe('dev')
    expect(wrapper.find('[data-testid="about-stat-commit"]').text()).toContain('Not stamped')
    expect(wrapper.find('[data-testid="about-update-status"]').text()).toBe('Unreleased build')
  })

  it('copies the build as a summary worth pasting into an issue', async () => {
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="about-copy-build"]').trigger('click')
    expect(mocks.SetText).toHaveBeenCalledWith(
      'Hive Desktop 1.4.0 (stable)\nCommit abc1234 · built 2026-07-01T12:00:00Z\ndarwin/arm64 · go1.24.2',
    )
  })

  it('toggles automatic updates through the service', async () => {
    mocks.Status.mockResolvedValue(updateInfo({ enabled: true }))
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="about-auto-update"]').trigger('click')
    expect(mocks.SetEnabled).toHaveBeenCalledWith(false)
  })

  it('checks for updates and shows an available result inline', async () => {
    mocks.CheckNow.mockResolvedValue(
      updateInfo({ available: true, latestVersion: '1.5.0', notes: 'Faster feed rendering.' }),
    )
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="about-check-update"]').trigger('click')
    await flushPromises()
    expect(mocks.CheckNow).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="about-update-available"]').text()).toContain('1.5.0')
    expect(wrapper.find('[data-testid="about-update-notes"]').text()).toBe('Faster feed rendering.')
    expect(wrapper.find('[data-testid="about-update-status"]').text()).toBe('Update available')
  })

  it('reports when the last check landed, background polls included', async () => {
    const checkedAt = new Date(Date.now() - 5 * 60 * 1000).toISOString()
    mocks.Status.mockResolvedValue(updateInfo({ checkedAt }))
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    expect(wrapper.find('[data-testid="about-update-status"]').text()).toBe('Up to date')
    expect(wrapper.find('[data-testid="about-update-checked"]').text()).toBe('Checked 5 minutes ago.')
  })

  it('restores the switch when persisting automatic updates fails', async () => {
    mocks.SetEnabled.mockRejectedValue(new Error('disk is read-only'))
    const wrapper = mount(AboutSettingsView)
    await flushPromises()

    await wrapper.find('[data-testid="about-auto-update"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="about-auto-update"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.find('[data-testid="about-error"]').text()).toContain('disk is read-only')
  })
})
