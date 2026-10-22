import { defineComponent, h, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useDrivePaneRequests } from '../useDrivePaneRequests'

const mocks = vi.hoisted(() => ({
  ack: vi.fn().mockResolvedValue(undefined),
  listener: null as null | ((event: { data: unknown }) => void),
}))
vi.mock('../useWailsEvent', () => ({
  useWailsEvent: (_: string, handler: typeof mocks.listener) => {
    mocks.listener = handler
  },
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/driveservice', () => ({
  AcknowledgePane: mocks.ack,
}))

describe('Drive pane requests', () => {
  beforeEach(() => {
    mocks.ack.mockClear()
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockReturnValue({ width: 380 } as DOMRect)
  })

  function setup(session: string | null) {
    const current = ref(session)
    const open = ref(false)
    const navigate = vi.fn(async (value: boolean, _path?: string) => {
      open.value = value
    })
    const wrapper = mount(
      defineComponent({
        setup() {
          useDrivePaneRequests(() => current.value, navigate)
          return () => (open.value ? h('aside', { 'data-testid': 'drive-pane' }) : null)
        },
      }),
      { attachTo: document.body },
    )
    return { current, navigate, wrapper }
  }

  it.each(['session', 'hiveSession'])('opens a collapsed pane and acknowledges after mount for %s', async (key) => {
    const { wrapper, navigate } = setup('abc')
    expect(document.querySelector('[data-testid="drive-pane"]')).toBeNull()
    mocks.listener!({ data: { requestId: 'open', [key]: 'abc', open: true, path: 'canvases/report.canvas.json' } })
    await flushPromises()
    expect(navigate).toHaveBeenCalledWith(true, 'canvases/report.canvas.json')
    expect(document.querySelector('[data-testid="drive-pane"]')).not.toBeNull()
    expect(mocks.ack).toHaveBeenLastCalledWith('open', 'opened')
    mocks.listener!({ data: [{ requestId: 'close', [key]: 'abc', open: false }] })
    await flushPromises()
    expect(document.querySelector('[data-testid="drive-pane"]')).toBeNull()
    expect(mocks.ack).toHaveBeenLastCalledWith('close', 'closed')
    wrapper.unmount()
  })

  it.each([null, 'another'])('reports a session not in view without navigation (%s)', async (session) => {
    const { wrapper, navigate } = setup(session)
    mocks.listener!({ data: { requestId: 'other', hiveSession: 'abc', open: true } })
    await flushPromises()
    expect(navigate).not.toHaveBeenCalled()
    expect(mocks.ack).toHaveBeenCalledWith('other', 'session_not_visible')
    wrapper.unmount()
  })

  it('does not claim success when navigation leaves the pane hidden', async () => {
    const { wrapper, navigate } = setup('abc')
    navigate.mockImplementation(async () => {})
    mocks.listener!({ data: { requestId: 'hidden', hiveSession: 'abc', open: true } })
    await flushPromises()
    expect(mocks.ack).toHaveBeenCalledWith('hidden', 'navigation_failed')
    wrapper.unmount()
  })

  it('does not claim success after the user switches sessions', async () => {
    const { wrapper, navigate, current } = setup('abc')
    navigate.mockImplementation(async () => {
      current.value = 'another'
    })
    mocks.listener!({ data: { requestId: 'switched', hiveSession: 'abc', open: true } })
    await flushPromises()
    expect(mocks.ack).toHaveBeenCalledWith('switched', 'session_not_visible')
    wrapper.unmount()
  })
})
