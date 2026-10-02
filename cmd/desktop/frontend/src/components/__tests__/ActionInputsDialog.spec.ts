import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ActionInputsDialog from '../ActionInputsDialog.vue'
import type { InputSpec } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/actions/models'

function spec(overrides: Partial<InputSpec> = {}): InputSpec {
  return {
    name: 'reason',
    label: 'Reason',
    type: 'text',
    required: true,
    default: '',
    placeholder: 'why',
    options: null,
    ...overrides,
  }
}

function mountDialog(inputs: InputSpec[]) {
  return mount(ActionInputsDialog, {
    attachTo: document.body,
    props: { actionLabel: 'Silence alert', inputs, busy: false, error: null },
    global: { stubs: { Teleport: true } },
  })
}

describe('ActionInputsDialog', () => {
  it('prefills declared defaults and emits the collected values', async () => {
    const wrapper = mountDialog([spec(), spec({ name: 'window', label: 'Window', required: false, default: '1h' })])
    await wrapper.get('[data-testid="action-input-reason"]').setValue('flapping')
    await wrapper.get('[data-testid="action-inputs-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toEqual([[{ reason: 'flapping', window: '1h' }]])
  })

  it('keeps the dialog open and names the missing required input', async () => {
    const wrapper = mountDialog([spec()])
    await wrapper.get('[data-testid="action-inputs-submit"]').trigger('click')
    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(wrapper.get('[data-testid="action-inputs-error"]').text()).toContain('Reason is required')
  })

  it('renders a multiline input as a textarea', () => {
    const wrapper = mountDialog([spec({ type: 'multiline' })])
    expect(wrapper.get('[data-testid="action-input-reason"]').element.tagName).toBe('TEXTAREA')
  })
})
