import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { defaults, validate, type Config } from '../config'

function fire(el: Element, type: string) {
  el.dispatchEvent(new Event(type, { bubbles: true }))
}

describe('function editor', () => {
  it('shows the on_message code field by default', () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    expect(wrapper.find('[data-testid="function-editor-on-message"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="function-editor-on-start"]').exists()).toBe(false)
  })

  it('emits an immutable update:config on an on_message edit, without mutating the config prop', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    const textarea = wrapper.get<HTMLTextAreaElement>('[data-testid="function-editor-on-message"]').element
    textarea.value = 'return null'
    fire(textarea, 'input')
    await wrapper.vm.$nextTick()

    expect(config.on_message).toBe('return msg')
    expect(wrapper.emitted('update:config')).toEqual([[{ on_message: 'return null' }]])
  })

  it('switches to the on_start tab and edits it independently', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    await wrapper.get('[data-testid="function-editor-tab-on_start"]').trigger('click')
    expect(wrapper.find('[data-testid="function-editor-on-start"]').exists()).toBe(true)

    const textarea = wrapper.get<HTMLTextAreaElement>('[data-testid="function-editor-on-start"]').element
    textarea.value = 'state.count = 0'
    fire(textarea, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ on_message: 'return msg', on_start: 'state.count = 0' }]])
  })

  it('edits outputs as a number', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="function-editor-outputs"]').element
    input.value = '3'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ on_message: 'return msg', outputs: 3 }]])
  })

  it('displays the default timeout as "5s" and parses a typed duration back into milliseconds', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    expect(wrapper.get<HTMLInputElement>('[data-testid="function-editor-timeout"]').element.value).toBe('5s')

    const input = wrapper.get<HTMLInputElement>('[data-testid="function-editor-timeout"]').element
    input.value = '10s'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toEqual([[{ on_message: 'return msg', timeout: 10000 }]])
  })

  it('ignores an unparsable timeout instead of emitting a bad value', async () => {
    const config: Config = { on_message: 'return msg' }
    const wrapper = mount(Editor, { props: { config } })

    const input = wrapper.get<HTMLInputElement>('[data-testid="function-editor-timeout"]').element
    input.value = 'not a duration'
    fire(input, 'input')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:config')).toBeUndefined()
  })
})

describe('function validate', () => {
  it('requires on_message', () => {
    expect(validate({ on_message: '' })).toEqual(['on_message is required'])
  })

  it('surfaces a syntax error from on_message', () => {
    const errors = validate({ on_message: 'return msg(' })
    expect(errors).toHaveLength(1)
    expect(errors[0]).toEqual(expect.any(String))
  })

  it('passes for valid source with defaults', () => {
    expect(validate(defaults)).toEqual([])
  })

  it('flags outputs out of the 1..16 range', () => {
    expect(validate({ on_message: 'return msg', outputs: 0 })).toContain('outputs must be between 1 and 16')
    expect(validate({ on_message: 'return msg', outputs: 17 })).toContain('outputs must be between 1 and 16')
  })

  it('flags timeout out of the 100ms..60s range', () => {
    expect(validate({ on_message: 'return msg', timeout: 50 })).toContain('timeout must be between 100ms and 60s')
    expect(validate({ on_message: 'return msg', timeout: 70000 })).toContain('timeout must be between 100ms and 60s')
  })
})
