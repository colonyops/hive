import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import PopOutHost from '../PopOutHost.vue'

const api = vi.hoisted(() => ({
  Retarget: vi.fn(() => Promise.resolve()),
  SetTitle: vi.fn(() => Promise.resolve()),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/popoutservice', () => ({
  Retarget: api.Retarget,
}))
vi.mock('@wailsio/runtime', () => ({ Window: { SetTitle: api.SetTitle } }))

const Fake = defineComponent({
  props: { params: { type: Object, required: true } },
  emits: ['update:params', 'title'],
  setup(props, { emit }) {
    return () =>
      h(
        'button',
        {
          'data-testid': 'fake',
          onClick: () => {
            emit('update:params', { doc: 'b.md' })
            emit('title', 'B')
          },
        },
        JSON.stringify(props.params),
      )
  },
})
vi.mock('../surfaces', () => ({ popOutSurfaces: { fake: () => Promise.resolve(Fake) } }))

describe('PopOutHost', () => {
  beforeEach(() => vi.clearAllMocks())
  afterEach(() => window.history.replaceState(null, '', '/'))

  it('hands the surface the URL params without the kind', async () => {
    window.history.replaceState(null, '', '/?popout=fake&doc=a.md')
    const wrapper = mount(PopOutHost)
    await flushPromises()

    expect(wrapper.get('[data-testid="fake"]').text()).toBe('{"doc":"a.md"}')
  })

  it('keeps the URL, the service, and the title in step with the surface', async () => {
    window.history.replaceState(null, '', '/?popout=fake&doc=a.md')
    const wrapper = mount(PopOutHost)
    await flushPromises()

    await wrapper.get('[data-testid="fake"]').trigger('click')

    expect(wrapper.get('[data-testid="fake"]').text()).toBe('{"doc":"b.md"}')
    expect(window.location.search).toBe('?popout=fake&doc=b.md')
    expect(api.Retarget).toHaveBeenCalledWith({ kind: 'fake', params: { doc: 'b.md' } })
    expect(api.SetTitle).toHaveBeenCalledWith('B')
  })

  it('says so when no surface has the kind', async () => {
    window.history.replaceState(null, '', '/?popout=nope')
    const wrapper = mount(PopOutHost)
    await flushPromises()

    expect(wrapper.get('[data-testid="popout-unknown"]').text()).toContain('nope')
  })
})
