import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import BaseBadge from '../BaseBadge.vue'

describe('BaseBadge', () => {
  it.each([
    ['neutral', 'bg-chip', 'text-text-3'],
    ['success', 'bg-severity-success-tint', 'text-severity-success'],
    ['accent', 'bg-accent-tint', 'text-accent'],
    ['muted', 'bg-chip', 'text-text-4'],
    ['danger', 'bg-severity-error-tint', 'text-severity-error'],
  ] as const)('applies %s tone colors', (tone, background, text) => {
    const wrapper = mount(BaseBadge, { props: { tone } })

    expect(wrapper.classes()).toEqual(expect.arrayContaining([background, text]))
  })

  it('uses chip rounding by default and pill rounding when requested', () => {
    expect(mount(BaseBadge).classes()).toContain('rounded-[5px]')
    expect(mount(BaseBadge, { props: { variant: 'pill' } }).classes()).toContain('rounded-full')
  })

  it('renders an optional leading status dot and its label slot', () => {
    const wrapper = mount(BaseBadge, { props: { tone: 'success', dot: true }, slots: { default: 'Connected' } })

    expect(wrapper.find('[aria-hidden="true"]').classes()).toEqual(expect.arrayContaining(['size-1.5', 'bg-severity-success']))
    expect(wrapper.text()).toBe('Connected')
  })

  it('forwards attributes and merges caller classes onto its root', () => {
    const wrapper = mount(BaseBadge, { attrs: { class: 'px-2 py-0.5 text-[11px]', 'data-testid': 'badge' } })

    expect(wrapper.attributes('data-testid')).toBe('badge')
    expect(wrapper.classes()).toEqual(expect.arrayContaining(['px-2', 'py-0.5', 'text-[11px]']))
  })
})
