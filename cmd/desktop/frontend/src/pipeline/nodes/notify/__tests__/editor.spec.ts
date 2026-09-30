import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Editor from '../editor.vue'
import { bodyMaxLen, defaultCooldownSeconds, defaults, titleMaxLen, validate } from '../config'
import { chooseOption } from '../../../../test-utils/select'

describe('notify editor', () => {
  it('renders the notify node body with its template and delivery fields', () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    expect(wrapper.get('[data-testid="notify-node-editor"]').text()).toContain('notification settings always win')
    for (const field of ['title', 'body', 'severity', 'sound', 'cooldown']) {
      expect(wrapper.find(`[data-testid="notify-node-editor-${field}"]`).exists(), field).toBe(true)
    }
  })

  it('emits the typed title template', async () => {
    const wrapper = mount(Editor, { props: { config: defaults } })
    await wrapper.get('[data-testid="notify-node-editor-title"]').setValue('{{ .Payload.repo }}')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: '{{ .Payload.repo }}' })
  })

  it('clears an emptied body back to undefined', async () => {
    const wrapper = mount(Editor, { props: { config: { title: 'hi', body: 'x' } } })
    await wrapper.get('[data-testid="notify-node-editor-body"]').setValue('')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', body: undefined })
  })

  // Only a non-default choice is persisted, so a flow file stays free of keys
  // the author never touched.
  it('stores severity only when it differs from the default', async () => {
    const wrapper = mount(Editor, { props: { config: { title: 'hi' } } })
    await chooseOption(wrapper, 'notify-node-editor-severity', 'warning')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', severity: 'warning' })
    wrapper.unmount()

    const warned = mount(Editor, { props: { config: { title: 'hi', severity: 'warning' } } })
    await chooseOption(warned, 'notify-node-editor-severity', 'info')
    expect(warned.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', severity: undefined })
    warned.unmount()
  })

  it('stores sound only when the node is silenced', async () => {
    const wrapper = mount(Editor, { props: { config: { title: 'hi' } } })
    await wrapper.get('[data-testid="notify-node-editor-sound"]').setValue(false)
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', sound: false })

    const silenced = mount(Editor, { props: { config: { title: 'hi', sound: false } } })
    await silenced.get('[data-testid="notify-node-editor-sound"]').setValue(true)
    expect(silenced.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', sound: undefined })
  })

  it('stores the cooldown only when it differs from the default; 0 disables', async () => {
    const wrapper = mount(Editor, { props: { config: { title: 'hi' } } })
    await wrapper.get('[data-testid="notify-node-editor-cooldown"]').setValue(60)
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', cooldownSeconds: 60 })

    // Clearing the field disables the cooldown explicitly rather than
    // silently restoring the default.
    await wrapper.get('[data-testid="notify-node-editor-cooldown"]').setValue('')
    expect(wrapper.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', cooldownSeconds: 0 })

    const configured = mount(Editor, { props: { config: { title: 'hi', cooldownSeconds: 60 } } })
    await configured.get('[data-testid="notify-node-editor-cooldown"]').setValue(defaultCooldownSeconds)
    expect(configured.emitted('update:config')?.at(-1)?.[0]).toEqual({ title: 'hi', cooldownSeconds: undefined })
  })
})

describe('notify config', () => {
  it('requires a title', () => {
    expect(validate(defaults)).toHaveLength(1)
    expect(validate({ title: '   ' })).toHaveLength(1)
    expect(validate({ title: '{{ .Payload.title }}' })).toEqual([])
  })

  it('rejects over-long templates', () => {
    expect(validate({ title: 'x'.repeat(titleMaxLen + 1) })).toHaveLength(1)
    expect(validate({ title: 'hi', body: 'x'.repeat(bodyMaxLen + 1) })).toHaveLength(1)
  })

  it('rejects an unsupported severity', () => {
    expect(validate({ title: 'hi', severity: 'critical' })).toHaveLength(1)
    expect(validate({ title: 'hi', severity: 'error' })).toEqual([])
  })

  it('rejects a negative or fractional cooldown', () => {
    expect(validate({ title: 'hi', cooldownSeconds: -1 })).toHaveLength(1)
    expect(validate({ title: 'hi', cooldownSeconds: 1.5 })).toHaveLength(1)
    expect(validate({ title: 'hi', cooldownSeconds: 0 })).toEqual([])
    expect(validate({ title: 'hi', cooldownSeconds: 300 })).toEqual([])
  })
})
