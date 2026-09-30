import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProfileSettingsView from '../ProfileSettingsView.vue'

const profile = {
  id: 'personal',
  letter: 'P',
  name: 'Personal',
  enabled: true,
  nodes: 0,
  sourceSummary: '2 sources',
  totalCount: 3,
  unreadCount: 1,
  feeds: [],
}

describe('ProfileSettingsView', () => {
  it('shows profile details and keeps delete in the danger zone', async () => {
    const wrapper = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })

    expect((wrapper.get('[data-testid="profile-settings-name"]').element as HTMLInputElement).value).toBe('Personal')
    expect(wrapper.find('[data-testid="profile-settings-delete"]').exists()).toBe(false)

    await wrapper.find('[data-testid="profile-settings-danger"]').trigger('click')
    expect(wrapper.emitted('select-section')).toEqual([['danger']])
    await wrapper.setProps({ activeSection: 'danger' })
    await wrapper.find('[data-testid="profile-settings-delete"]').trigger('click')

    expect(wrapper.emitted('delete')).toHaveLength(1)
  })

  it('is where the source count is shown, now that the sidebar header omits it', () => {
    const wrapper = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })
    expect(wrapper.get('[data-testid="profile-settings-sources"]').text()).toBe('2 sources')
  })

  it('edits and submits the profile name', async () => {
    const wrapper = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })
    const input = wrapper.get('[data-testid="profile-settings-name"]')
    const save = wrapper.get('[data-testid="profile-settings-save-name"]')

    expect(save.attributes('disabled')).toBeDefined()
    await input.setValue('Team Triage')
    expect(save.attributes('disabled')).toBeUndefined()
    await wrapper.get('form').trigger('submit')

    expect(wrapper.emitted('rename')).toEqual([['Team Triage']])
  })

  it('toggles profile polling and shows failures', async () => {
    const wrapper = mount(ProfileSettingsView, {
      props: { profile, activeSection: 'general', toggleError: 'Could not update' },
    })

    const toggle = wrapper.get('[data-testid="profile-settings-enabled"]')
    expect(toggle.attributes('aria-checked')).toBe('true')
    await toggle.trigger('click')
    expect(wrapper.emitted('toggle-enabled')).toEqual([[false]])
    expect(wrapper.get('[data-testid="profile-settings-toggle-error"]').text()).toBe('Could not update')

    await wrapper.setProps({ toggling: true })
    expect(toggle.attributes('disabled')).toBeDefined()
  })

  it('shows rename progress and errors', async () => {
    const wrapper = mount(ProfileSettingsView, {
      props: { profile, activeSection: 'general', renaming: true, renameError: 'Could not save' },
    })

    expect(wrapper.get('[data-testid="profile-settings-save-name"]').text()).toBe('Saving…')
    expect(wrapper.get('[data-testid="profile-settings-name"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="profile-settings-rename-error"]').text()).toBe('Could not save')
  })

  it('closes on Escape', async () => {
    const wrapper = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('offers upload with no image and preview + remove with one', async () => {
    const noImage = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })
    expect(noImage.get('[data-testid="profile-settings-image-upload"]').text()).toBe('Upload image')
    expect(noImage.find('[data-testid="profile-settings-image-remove"]').exists()).toBe(false)
    expect(noImage.find('[data-testid="profile-settings-image-preview"] img').exists()).toBe(false)

    const withImage = mount(ProfileSettingsView, {
      props: { profile: { ...profile, image: 'data:image/png;base64,AAAA' }, activeSection: 'general' },
    })
    expect(withImage.get('[data-testid="profile-settings-image-upload"]').text()).toBe('Replace image')
    expect(withImage.get('[data-testid="profile-settings-image-preview"] img').attributes('src')).toBe('data:image/png;base64,AAAA')
    await withImage.get('[data-testid="profile-settings-image-remove"]').trigger('click')
    expect(withImage.emitted('clear-image')).toHaveLength(1)
  })

  it('surfaces a rejected upload from the backend', () => {
    const wrapper = mount(ProfileSettingsView, {
      props: { profile, activeSection: 'general', imageError: 'That image is too large.' },
    })
    expect(wrapper.get('[data-testid="profile-settings-image-error"]').text()).toBe('That image is too large.')
  })

  it('reads a picked file and emits it as base64', async () => {
    const wrapper = mount(ProfileSettingsView, { props: { profile, activeSection: 'general' } })
    const input = wrapper.get('[data-testid="profile-settings-image-input"]')
    const file = new File([Uint8Array.from([1, 2, 3, 4])], 'avatar.png', { type: 'image/png' })
    Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
    await input.trigger('change')

    await vi.waitFor(() => expect(wrapper.emitted('set-image')).toHaveLength(1))
    const [[data]] = wrapper.emitted('set-image') as [string][]
    expect(typeof data).toBe('string')
    expect(data.length).toBeGreaterThan(0)
  })
})
