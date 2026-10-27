import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Spinner from '../Spinner.vue'

describe('Spinner', () => {
  it('is hidden from assistive tech, since its host announces the wait', () => {
    const wrapper = mount(Spinner)

    expect(wrapper.attributes('aria-hidden')).toBe('true')
  })
})
