import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SourceMark from '../SourceMark.vue'

// A stand-in glyph so the fallback branch is identifiable in the DOM.
const Glyph = { template: '<svg data-testid="glyph" />' }

describe('SourceMark', () => {
  it('renders the glyph when no image is set', () => {
    const wrapper = mount(SourceMark, { props: { icon: Glyph } })
    expect(wrapper.find('[data-testid="glyph"]').exists()).toBe(true)
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('renders the image over the glyph when one is provided', () => {
    const src = 'data:image/png;base64,LOGO'
    const wrapper = mount(SourceMark, { props: { icon: Glyph, image: src } })
    expect(wrapper.find('img').attributes('src')).toBe(src)
    expect(wrapper.find('[data-testid="glyph"]').exists()).toBe(false)
  })

  it('falls back to the glyph when the image fails to load', async () => {
    const wrapper = mount(SourceMark, { props: { icon: Glyph, image: 'data:image/png;base64,BROKEN' } })
    await wrapper.find('img').trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('[data-testid="glyph"]').exists()).toBe(true)
  })

  // Both branches must land at the size the badge asked for. `size-full` on the
  // image outranked the caller's `size-4` in the utility layer, so a Grafana or
  // uploaded mark filled the badge edge to edge and a failed image snapped down
  // to the glyph's size.
  it('renders the image at the size the caller passes, like the glyph', async () => {
    const wrapper = mount(SourceMark, { props: { icon: Glyph, image: 'data:image/png;base64,LOGO' }, attrs: { class: 'size-4' } })
    expect(wrapper.find('img').classes()).toContain('size-4')
    expect(wrapper.find('img').classes()).not.toContain('size-full')

    await wrapper.find('img').trigger('error')
    expect(wrapper.find('[data-testid="glyph"]').classes()).toContain('size-4')
  })

  it('retries the image after the source changes to a new one', async () => {
    const wrapper = mount(SourceMark, { props: { icon: Glyph, image: 'data:image/png;base64,BROKEN' } })
    await wrapper.find('img').trigger('error')
    expect(wrapper.find('img').exists()).toBe(false)

    await wrapper.setProps({ image: 'data:image/png;base64,FRESH' })
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,FRESH')
  })
})
