import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, descriptionMaxLen, validate } from '../config'
import { chooseOption } from '../../../../test-utils/select'

describe('feed editor', () => {
  it('renders the feed node body with icon and description fields', () => {
    const wrapper = mount(Editor, { props: { config: {} } })
    expect(wrapper.get('[data-testid="feed-node-editor"]').text()).toContain('FEEDS')
    expect(wrapper.find('[data-testid="feed-editor-icon"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="feed-editor-description"]').exists()).toBe(true)
  })

  it('emits the chosen icon from the searchable picker', async () => {
    const wrapper = mount(Editor, { props: { config: {} } })
    await chooseOption(wrapper, 'feed-editor-icon', 'sparkles')
    const events = wrapper.emitted('update:config')
    expect(events?.at(-1)?.[0]).toEqual({ icon: 'sparkles' })
    wrapper.unmount()
  })

  it('emits the typed description', async () => {
    const wrapper = mount(Editor, { props: { config: {} } })
    await wrapper.get('[data-testid="feed-editor-description"]').setValue('Team PRs')
    const events = wrapper.emitted('update:config')
    expect(events?.at(-1)?.[0]).toEqual({ description: 'Team PRs' })
  })

  it('clears a field back to undefined when emptied', async () => {
    const wrapper = mount(Editor, { props: { config: { description: 'x' } } })
    await wrapper.get('[data-testid="feed-editor-description"]').setValue('')
    const events = wrapper.emitted('update:config')
    expect(events?.at(-1)?.[0]).toEqual({ description: undefined })
  })

  it('carries no notification controls — a feed never interrupts', () => {
    const wrapper = mount(Editor, { props: { config: {} } })
    expect(wrapper.find('[data-testid="feed-editor-notify"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="feed-editor-notify-options"]').exists()).toBe(false)
  })
})

describe('feed config', () => {
  it('accepts empty config', () => {
    expect(validate(defaults)).toEqual([])
  })

  it('accepts a supported icon', () => {
    expect(validate({ icon: 'sparkles' })).toEqual([])
  })

  it('rejects an unsupported icon', () => {
    expect(validate({ icon: 'not-an-icon' })).toHaveLength(1)
  })

  it('rejects an over-long description', () => {
    expect(validate({ description: 'x'.repeat(descriptionMaxLen + 1) })).toHaveLength(1)
  })
})
