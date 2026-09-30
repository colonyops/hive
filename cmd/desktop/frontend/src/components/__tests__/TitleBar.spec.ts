import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TitleBar from '../TitleBar.vue'

describe('TitleBar', () => {
  it('renders no profile controls during onboarding (no profile)', () => {
    const wrapper = mount(TitleBar, { props: {} })
    expect(wrapper.find('[data-testid="titlebar-toggle-sidebar"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="titlebar-activity"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="titlebar-command-palette"]').exists()).toBe(false)
  })

  it('shows the Activity icon once a profile is loaded and emits open-activity on click', async () => {
    const onboarding = mount(TitleBar, { props: {} })
    expect(onboarding.find('[data-testid="titlebar-activity"]').exists()).toBe(false)

    const wrapper = mount(TitleBar, { props: { profileName: 'Triage' } })
    const link = wrapper.find('[data-testid="titlebar-activity"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('aria-label')).toBe('Open activity')

    await link.trigger('click')
    expect(wrapper.emitted('open-activity')).toHaveLength(1)
  })

  it('shows the unseen-activity dot only with unseen events and not while on the Activity page', () => {
    const unseen = mount(TitleBar, { props: { profileName: 'Triage', unseenActivity: 3 } })
    expect(unseen.find('[data-testid="titlebar-activity-unseen"]').exists()).toBe(true)

    const none = mount(TitleBar, { props: { profileName: 'Triage', unseenActivity: 0 } })
    expect(none.find('[data-testid="titlebar-activity-unseen"]').exists()).toBe(false)

    const active = mount(TitleBar, { props: { profileName: 'Triage', unseenActivity: 3, activityActive: true } })
    expect(active.find('[data-testid="titlebar-activity-unseen"]').exists()).toBe(false)
  })

  it('shows live jobs, opens the popover, and emits the selected command', async () => {
    const job = {
      id: 7, createdAt: 1, updatedAt: 2, status: 'done', label: 'Review PR',
      step: 'Completed', actionId: 'review', target: 'pr-1', commandId: 42,
    }
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', jobsActive: true, activeJobs: [job] } })
    const chip = wrapper.find('[data-testid="titlebar-jobs"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('1 job')
    await chip.trigger('click')
    expect(wrapper.find('[data-testid="jobs-popover"]').exists()).toBe(true)
    await wrapper.find('[data-testid="job-open-run-7"]').trigger('click')
    expect(wrapper.emitted('open-job-run')).toEqual([[42]])

    await wrapper.setProps({ jobsActive: false })
    expect(wrapper.find('[data-testid="titlebar-jobs"]').exists()).toBe(false)
  })

  it('exposes enabled back and forward history controls', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', canGoBack: true, canGoForward: true } })

    await wrapper.find('[data-testid="titlebar-back"]').trigger('click')
    await wrapper.find('[data-testid="titlebar-forward"]').trigger('click')

    expect(wrapper.emitted('back')).toHaveLength(1)
    expect(wrapper.emitted('forward')).toHaveLength(1)
  })

  it('hides the history and command-palette controls during onboarding', () => {
    const onboarding = mount(TitleBar, { props: {} })
    expect(onboarding.find('[data-testid="titlebar-back"]').exists()).toBe(false)
    expect(onboarding.find('[data-testid="titlebar-command-palette"]').exists()).toBe(false)

    const loaded = mount(TitleBar, { props: { profileName: 'Triage' } })
    expect(loaded.find('[data-testid="titlebar-command-palette"]').exists()).toBe(true)
  })

  it('opens the command palette from the launcher in the right cluster', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage' } })
    await wrapper.find('[data-testid="titlebar-command-palette"]').trigger('click')
    expect(wrapper.emitted('open-palette')).toHaveLength(1)
  })

  it('keeps the sidebar toggle mounted with a profile, disabling it when no sidebar is available', async () => {
    const disabled = mount(TitleBar, { props: { profileName: 'Triage' } })
    const disabledToggle = disabled.find('[data-testid="titlebar-toggle-sidebar"]')
    expect(disabledToggle.exists()).toBe(true)
    expect(disabledToggle.attributes('disabled')).toBeDefined()

    const expanded = mount(TitleBar, { props: { profileName: 'Triage', canToggleSidebar: true } })
    const toggle = expanded.find('[data-testid="titlebar-toggle-sidebar"]')
    expect(toggle.attributes('disabled')).toBeUndefined()
    expect(toggle.attributes('aria-label')).toBe('Hide sidebar')

    await toggle.trigger('click')
    expect(expanded.emitted('toggle-sidebar')).toHaveLength(1)

    const collapsed = mount(TitleBar, { props: { profileName: 'Triage', canToggleSidebar: true, sidebarCollapsed: true } })
    expect(collapsed.find('[data-testid="titlebar-toggle-sidebar"]').attributes('aria-label')).toBe('Show sidebar')
  })

  it('keeps the preview toggle mounted with a profile, disabling it outside the feed view', async () => {
    const disabled = mount(TitleBar, { props: { profileName: 'Triage' } })
    const disabledToggle = disabled.find('[data-testid="titlebar-toggle-preview"]')
    expect(disabledToggle.exists()).toBe(true)
    expect(disabledToggle.attributes('disabled')).toBeDefined()

    const expanded = mount(TitleBar, { props: { profileName: 'Triage', canTogglePreview: true } })
    const toggle = expanded.find('[data-testid="titlebar-toggle-preview"]')
    expect(toggle.attributes('disabled')).toBeUndefined()
    expect(toggle.attributes('aria-label')).toBe('Hide preview')

    await toggle.trigger('click')
    expect(expanded.emitted('toggle-preview')).toHaveLength(1)

    const collapsed = mount(TitleBar, { props: { profileName: 'Triage', canTogglePreview: true, previewCollapsed: true } })
    expect(collapsed.find('[data-testid="titlebar-toggle-preview"]').attributes('aria-label')).toBe('Show preview')
  })

  it('zooms on a double-click of the bar itself, but not on its controls', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', canToggleSidebar: true } })

    await wrapper.find('header').trigger('dblclick')
    expect(wrapper.emitted('toggle-maximise')).toHaveLength(1)

    await wrapper.find('[data-testid="titlebar-command-palette"]').trigger('dblclick')
    expect(wrapper.emitted('toggle-maximise')).toHaveLength(1)
  })

  it('renders the update chip only when updateAvailable and emits open-update on click', async () => {
    const none = mount(TitleBar, { props: { profileName: 'Triage' } })
    expect(none.find('[data-testid="titlebar-update-chip"]').exists()).toBe(false)

    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', updateAvailable: true, latestVersion: '1.5.0' } })
    const chip = wrapper.find('[data-testid="titlebar-update-chip"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('1.5.0')

    await chip.trigger('click')
    expect(wrapper.emitted('open-update')).toHaveLength(1)
  })

  it('shows install progress and disables repeated update clicks', async () => {
    const wrapper = mount(TitleBar, { props: { updateAvailable: true, updateInstalling: true, latestVersion: '1.5.0' } })
    const chip = wrapper.get('[data-testid="titlebar-update-chip"]')

    expect(chip.attributes('disabled')).toBeDefined()
    expect(chip.text()).toContain('Installing…')
    await chip.trigger('click')
    expect(wrapper.emitted('open-update')).toBeUndefined()
  })

  it('shows the update chip during onboarding (no profile)', () => {
    const wrapper = mount(TitleBar, { props: { updateAvailable: true, latestVersion: '2.0.0' } })
    expect(wrapper.find('[data-testid="titlebar-update-chip"]').exists()).toBe(true)
  })

  it('renders the error chip only when errorCount > 0 and emits open-error-node on click', async () => {
    const none = mount(TitleBar, { props: { profileName: 'Triage', errorCount: 0 } })
    expect(none.find('[data-testid="titlebar-error-chip"]').exists()).toBe(false)

    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', errorCount: 2 } })
    const chip = wrapper.find('[data-testid="titlebar-error-chip"]')
    expect(chip.exists()).toBe(true)
    expect(chip.text()).toContain('2 errors')

    await chip.trigger('click')
    expect(wrapper.emitted('open-error-node')).toHaveLength(1)
  })

  it('hides the whole mode group during onboarding, when there is no profile', () => {
    const wrapper = mount(TitleBar, { props: { mode: 'hub' } })
    expect(wrapper.find('[data-testid="titlebar-mode-hub"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="titlebar-mode-terminal"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="titlebar-mode-agents"]').exists()).toBe(false)
  })

  it('keeps the Inbox|Code toggle live in both modes and reports the pressed one', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', mode: 'hub' } })
    const hub = wrapper.get('[data-testid="titlebar-mode-hub"]')
    const terminal = wrapper.get('[data-testid="titlebar-mode-terminal"]')

    expect(hub.attributes('aria-pressed')).toBe('true')
    expect(terminal.attributes('aria-pressed')).toBe('false')
    // Always clickable: an unavailable terminal explains itself inside the mode.
    expect(terminal.attributes('disabled')).toBeUndefined()

    await terminal.trigger('click')
    expect(wrapper.emitted('set-mode')).toEqual([['terminal']])

    await wrapper.setProps({ mode: 'terminal' })
    expect(wrapper.get('[data-testid="titlebar-mode-terminal"]').attributes('aria-pressed')).toBe('true')
    await wrapper.get('[data-testid="titlebar-mode-hub"]').trigger('click')
    expect(wrapper.emitted('set-mode')).toEqual([['terminal'], ['hub']])
  })

  it('reports the pressed segment across the Agents mode too', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', mode: 'hub' } })
    const hub = wrapper.get('[data-testid="titlebar-mode-hub"]')
    const agents = wrapper.get('[data-testid="titlebar-mode-agents"]')

    expect(hub.attributes('aria-pressed')).toBe('true')
    expect(agents.attributes('aria-pressed')).toBe('false')

    await agents.trigger('click')
    expect(wrapper.emitted('set-mode')).toEqual([['agents']])

    await wrapper.setProps({ mode: 'agents' })
    expect(wrapper.get('[data-testid="titlebar-mode-agents"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="titlebar-mode-hub"]').attributes('aria-pressed')).toBe('false')
  })

  it('renders all three segments', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', mode: 'agents' } })
    const hub = wrapper.get('[data-testid="titlebar-mode-hub"]')
    const terminal = wrapper.get('[data-testid="titlebar-mode-terminal"]')
    const agents = wrapper.get('[data-testid="titlebar-mode-agents"]')

    expect(hub.attributes('aria-pressed')).toBe('false')
    expect(terminal.attributes('aria-pressed')).toBe('false')
    expect(agents.attributes('aria-pressed')).toBe('true')

    await terminal.trigger('click')
    expect(wrapper.emitted('set-mode')).toEqual([['terminal']])
  })
})
