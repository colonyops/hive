import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { beforeEach, expect, it, vi } from 'vitest'
import RemoteTerminal from '../RemoteTerminal.vue'

const mocks = vi.hoisted(() => ({ reconnect: vi.fn(), attach: vi.fn(), dispose: vi.fn(), select: vi.fn() }))
const status = ref('live')
const tabs = ref([{ windowId: '@1', uid: 'one', name: 'bash' }])
const remoteBusy = ref(false)
vi.mock('../../stores/useRemoteConnection', () => ({
  useRemoteConnection: () => ({
    reportStreamFailure: vi.fn(),
    reconnect: mocks.reconnect,
    busy: remoteBusy,
    error: ref(null),
  }),
}))
vi.mock('../../lib/terminalClient', () => ({ createTerminalClient: vi.fn() }))
vi.mock('../../composables/useTerminalWindows', () => ({
  useTerminalWindows: () => ({
    tabs,
    select: mocks.select,
    activeWindowId: ref('@1'),
    status,
    endReason: ref('disconnected'),
    error: ref('Terminal connection lost.'),
    actionError: ref(null),
    outputDropped: ref(false),
    start: mocks.attach,
    dispose: mocks.dispose,
  }),
}))
vi.mock('../TerminalTab.vue', () => ({ default: { template: '<div />' } }))
beforeEach(() => {
  tabs.value = [{ windowId: '@1', uid: 'one', name: 'bash' }]
  status.value = 'live'
  remoteBusy.value = false
  vi.clearAllMocks()
})

it('shows no pane toolbar while live and reconnects the server after disconnection', async () => {
  const wrapper = mount(RemoteTerminal, {
    props: { slug: 'demo', endpoint: { httpBaseURL: 'hive-remote://one', token: '', wsURL: '' } },
  })
  await flushPromises()
  expect(wrapper.find('[data-testid="remote-reconnect"]').exists()).toBe(false)
  expect(wrapper.text()).not.toContain('bash')
  status.value = 'ended'
  await flushPromises()
  await wrapper.get('[data-testid="remote-reconnect"]').trigger('click')
  expect(mocks.reconnect).toHaveBeenCalledOnce()
  wrapper.unmount()
  expect(mocks.dispose).toHaveBeenCalledOnce()
})

it('selects a requested window when its stream tab arrives after the sidebar selection', async () => {
  const wrapper = mount(RemoteTerminal, {
    props: { slug: 'demo', endpoint: { httpBaseURL: 'hive-remote://one', token: '', wsURL: '' } },
  })
  await flushPromises()
  await wrapper.setProps({ windowId: '@2' })
  expect(mocks.select).not.toHaveBeenCalled()
  tabs.value.push({ windowId: '@2', uid: 'two', name: 'codex' })
  await flushPromises()
  expect(mocks.select).toHaveBeenCalledWith('@2')
  wrapper.unmount()
})
