import { DOMWrapper, mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import TokenInput from '../TokenInput.vue'
import AppTooltip from '../AppTooltip.vue'

// The suggestion list is teleported to <body>.
const body = () => new DOMWrapper(document.body)

function mountInput(props: Partial<InstanceType<typeof TokenInput>['$props']> = {}) {
  return mount(TokenInput, {
    props: { modelValue: [], suggestions: ['auto', 'Backend'], testid: 'tokens', ...props },
    attachTo: document.body,
  })
}

describe('TokenInput', () => {
  it('adds the draft on Enter and on a comma', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('[data-testid="tokens"]')
    await input.setValue('review')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['review']])

    await wrapper.setProps({ modelValue: ['review'] })
    await input.setValue('urgent')
    await input.trigger('keydown', { key: ',' })
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([['review', 'urgent']])
    wrapper.unmount()
  })

  it('adds the typed text, not a suggestion it is part of, until the user moves to one', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('[data-testid="tokens"]')
    await input.setValue('au')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['au']])

    await input.setValue('au')
    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'ArrowUp' })
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([['auto']])
    wrapper.unmount()
  })

  it('takes the first match on Enter when autoPick is set', async () => {
    const wrapper = mountInput({ autoPick: true })
    const input = wrapper.get('[data-testid="tokens"]')
    await input.setValue('back')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['Backend']])
    wrapper.unmount()
  })

  it('leaves a modified Enter and an IME commit to the browser', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('[data-testid="tokens"]')
    await input.trigger('click')
    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'Enter', metaKey: true })
    await input.trigger('keydown', { key: 'Enter', isComposing: true })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  it('splits a pasted comma-separated list', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('[data-testid="tokens"]')
    await input.setValue('a, b,,A, backend')
    await input.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['a', 'b', 'Backend']])
    wrapper.unmount()
  })

  it("takes a suggestion's spelling and ignores a repeat in another case", async () => {
    const wrapper = mountInput({ modelValue: ['auto'] })
    const input = wrapper.get('[data-testid="tokens"]')
    await input.setValue('backend')
    await input.trigger('blur')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['auto', 'Backend']])

    await input.setValue('AUTO')
    await input.trigger('blur')
    expect(wrapper.emitted('update:modelValue')).toHaveLength(1)
    wrapper.unmount()
  })

  it('lists suggestions not already chosen, narrowed by the draft, and adds a picked one', async () => {
    const wrapper = mountInput({ modelValue: ['auto'] })
    const input = wrapper.get('[data-testid="tokens"]')
    await input.trigger('click')
    expect(body().find('[data-testid="tokens-option-auto"]').exists()).toBe(false)
    expect(body().find('[data-testid="tokens-option-Backend"]').exists()).toBe(true)

    await input.setValue('zzz')
    expect(body().find('[data-testid="tokens-suggestions"]').exists()).toBe(false)

    await input.setValue('back')
    await body().get('[data-testid="tokens-option-Backend"]').trigger('mousedown')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['auto', 'Backend']])
    wrapper.unmount()
  })

  it('keeps the list closed when the input is only focused', async () => {
    const wrapper = mountInput()
    await wrapper.get('[data-testid="tokens"]').trigger('focus')
    expect(body().find('[data-testid="tokens-suggestions"]').exists()).toBe(false)
    await wrapper.get('[data-testid="tokens"]').trigger('keydown', { key: 'ArrowDown' })
    expect(body().find('[data-testid="tokens-option-auto"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('removes the last token on Backspace and a token by its button', async () => {
    const wrapper = mountInput({ modelValue: ['auto', 'review'] })
    await wrapper.get('[data-testid="tokens"]').trigger('keydown', { key: 'Backspace' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['auto']])

    await wrapper.get('[data-testid="tokens-token-auto"] button').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[1]).toEqual([['review']])
    wrapper.unmount()
  })

  it('commits the draft on flush and explains a flagged token', async () => {
    const wrapper = mountInput({ modelValue: ['bogus'], flag: (token: string) => (token === 'bogus' ? 'Unknown' : '') })
    expect(wrapper.getComponent(AppTooltip).props('text')).toBe('Unknown')

    await wrapper.get('[data-testid="tokens"]').setValue('later')
    ;(wrapper.vm as unknown as { flush: () => void }).flush()
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([['bogus', 'later']])
    wrapper.unmount()
  })
})
