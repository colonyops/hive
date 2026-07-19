import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TitleBar from '../TitleBar.vue'

describe('TitleBar', () => {
  it('shows only the wordmark during onboarding (no profile)', () => {
    const wrapper = mount(TitleBar, { props: {} })
    expect(wrapper.text()).toContain('hive')
    expect(wrapper.find('[data-testid="breadcrumb-profile-name"]').exists()).toBe(false)
  })

  it('shows the polling indicator in feed mode, not flows mode', () => {
    const feed = mount(TitleBar, { props: { profileName: 'Triage', flowsActive: false } })
    expect(feed.find('[data-testid="polling-indicator"]').exists()).toBe(true)

    const flows = mount(TitleBar, { props: { profileName: 'Triage', flowsActive: true } })
    expect(flows.find('[data-testid="polling-indicator"]').exists()).toBe(false)
  })

  it('adds a Flows breadcrumb and exits via the profile crumb in flows mode', async () => {
    const wrapper = mount(TitleBar, { props: { profileName: 'Triage', flowsActive: true } })
    expect(wrapper.find('[data-testid="breadcrumb-flows"]').text()).toBe('Flows')

    await wrapper.find('[data-testid="breadcrumb-profile-name"]').trigger('click')
    expect(wrapper.emitted('exit-flows')).toHaveLength(1)
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
