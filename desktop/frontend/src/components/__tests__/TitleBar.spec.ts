import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TitleBar from '../TitleBar.vue'

describe('TitleBar', () => {
  it('shows only the wordmark during onboarding (no profile)', () => {
    const wrapper = mount(TitleBar, { props: {} })
    expect(wrapper.text()).toContain('hive')
    expect(wrapper.find('[data-testid="breadcrumb-profile-name"]').exists()).toBe(false)
  })

  it('shows the Activity link once a profile is loaded and emits open-activity on click', async () => {
    const onboarding = mount(TitleBar, { props: {} })
    expect(onboarding.find('[data-testid="titlebar-activity"]').exists()).toBe(false)

    const wrapper = mount(TitleBar, { props: { profileName: 'Triage' } })
    const link = wrapper.find('[data-testid="titlebar-activity"]')
    expect(link.exists()).toBe(true)
    expect(link.text()).toContain('Activity')

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

  it('adds a Flows breadcrumb and exits via the profile crumb in flows mode', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', flowsActive: true } })
    expect(wrapper.find('[data-testid="breadcrumb-flows"]').text()).toBe('Flows')

    await wrapper.find('[data-testid="breadcrumb-profile-name"]').trigger('click')
    expect(wrapper.emitted('exit-flows')).toHaveLength(1)
  })

  it('exposes enabled back and forward history controls', async () => {
    const wrapper = mount(TitleBar, { props: { canGoBack: true, canGoForward: true } })

    await wrapper.find('[data-testid="titlebar-back"]').trigger('click')
    await wrapper.find('[data-testid="titlebar-forward"]').trigger('click')

    expect(wrapper.emitted('back')).toHaveLength(1)
    expect(wrapper.emitted('forward')).toHaveLength(1)
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
})
