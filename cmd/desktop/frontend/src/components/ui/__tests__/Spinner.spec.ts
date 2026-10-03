import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Spinner from '../Spinner.vue'

describe('Spinner', () => {
  it('is hidden from assistive tech without a label', () => {
    const wrapper = mount(Spinner)

    expect(wrapper.attributes('aria-hidden')).toBe('true')
    expect(wrapper.attributes('role')).toBeUndefined()
  })

  it('announces itself as a status when labelled', () => {
    const wrapper = mount(Spinner, { props: { label: 'Loading sessions' } })

    expect(wrapper.attributes('role')).toBe('status')
    expect(wrapper.attributes('aria-label')).toBe('Loading sessions')
    expect(wrapper.attributes('aria-hidden')).toBeUndefined()
  })
})
