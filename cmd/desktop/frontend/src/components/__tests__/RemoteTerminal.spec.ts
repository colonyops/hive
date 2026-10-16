import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { beforeEach, expect, it, vi } from 'vitest'
import RemoteTerminal from '../RemoteTerminal.vue'

const mocks = vi.hoisted(() => ({ reconnect: vi.fn(), attach: vi.fn(), dispose: vi.fn() }))
const status = ref('live')
const remoteBusy = ref(false)
vi.mock('../../stores/useRemoteConnection', () => ({
  useRemoteConnection: () => ({ reconnect: mocks.reconnect, busy: remoteBusy, error: ref(null) }),
}))
vi.mock('../../lib/terminalClient', () => ({ createTerminalClient: vi.fn() }))
vi.mock('../../composables/useTerminalWindows', () => ({
  useTerminalWindows: () => ({
    tabs: ref([{ windowId: '@1', uid: 'one', name: 'bash' }]),
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
