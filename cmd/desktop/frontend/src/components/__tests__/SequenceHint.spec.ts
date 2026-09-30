import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SequenceHint from '../SequenceHint.vue'
import { useKeybindings } from '../../composables/useKeybindings'

const kb = useKeybindings()

afterEach(() => {
  kb.pendingSequence.value = null
})

describe('SequenceHint', () => {
  it('renders nothing while no sequence is pending', () => {
    const wrapper = mount(SequenceHint)

    expect(wrapper.find('[data-testid="sequence-hint"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it('renders the pressed steps as formatted keycaps', () => {
    kb.pendingSequence.value = {
      steps: ['g'],
      continuations: [{ step: 'i', commandId: 'view.go-inbox' }],
    }
    const wrapper = mount(SequenceHint)

    expect(wrapper.find('[data-testid="sequence-hint"]').exists()).toBe(true)
    const steps = wrapper.findAll('[data-testid="sequence-hint-step"]').map((el) => el.text())
    expect(steps).toEqual(['G'])

    wrapper.unmount()
  })

  it('resolves each continuation to its command title through the catalog', () => {
    kb.pendingSequence.value = {
      steps: ['g'],
      continuations: [
        { step: 'i', commandId: 'view.go-inbox' },
        { step: 'c', commandId: 'view.go-code' },
      ],
    }
    const wrapper = mount(SequenceHint)

    const continuations = wrapper.findAll('[data-testid="sequence-hint-continuation"]')
    expect(continuations).toHaveLength(2)
    expect(continuations[0].find('[data-testid="sequence-hint-continuation-key"]').text()).toBe('I')
    expect(continuations[0].find('[data-testid="sequence-hint-continuation-title"]').text()).toBe('Go to Inbox')
    expect(continuations[1].find('[data-testid="sequence-hint-continuation-key"]').text()).toBe('C')
    expect(continuations[1].find('[data-testid="sequence-hint-continuation-title"]').text()).toBe('Go to Code')

    wrapper.unmount()
  })

  it('skips a continuation whose commandId is not in the catalog', () => {
    kb.pendingSequence.value = {
      steps: ['g'],
      continuations: [
        { step: 'i', commandId: 'view.go-inbox' },
        { step: 'z', commandId: 'not.a.real.command' },
      ],
    }
    const wrapper = mount(SequenceHint)

    const continuations = wrapper.findAll('[data-testid="sequence-hint-continuation"]')
    expect(continuations).toHaveLength(1)
    expect(continuations[0].find('[data-testid="sequence-hint-continuation-title"]').text()).toBe('Go to Inbox')

    wrapper.unmount()
  })
})
