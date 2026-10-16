import { nextTick } from 'vue'
import { resetStores } from '../../stores/defineStore'
import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import RemoteSettingsView from '../RemoteSettingsView.vue'
import { useRemoteConnection } from '../../stores/useRemoteConnection'
import { useCodeInstallation } from '../../stores/useCodeInstallation'

const mocks = vi.hoisted(() => ({ list: vi.fn(), push: vi.fn() }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: mocks.push }) }))
vi.mock('../../lib/remoteConnection', async (original) => ({
  ...(await original<typeof import('../../lib/remoteConnection')>()),
  listRemoteSessions: mocks.list,
}))

it('connects from Settings and retains the connection after navigating to Code', async () => {
  mocks.list.mockResolvedValue([])
  const wrapper = mount(RemoteSettingsView)
  await wrapper.get('[data-testid="remote-name"]').setValue('Build machine')
  await wrapper.get('[data-testid="remote-color-purple"]').trigger('click')
  await wrapper.get('[data-testid="remote-host"]').setValue('hive-server')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(mocks.push).toHaveBeenCalledWith({ name: 'terminal' })
  expect(useCodeInstallation().installation.value).toBe('remote')
  expect(wrapper.find('[data-testid="remote-token"]').exists()).toBe(false)
  wrapper.unmount()
  expect(useRemoteConnection().endpoint.value?.token).toBe('')
  const reopened = mount(RemoteSettingsView)
  expect((reopened.get('[data-testid="remote-name"]').element as HTMLInputElement).value).toBe('Build machine')
  expect((reopened.get('[data-testid="remote-color"]').element as HTMLInputElement).value).toBe('#a855f7')
  await reopened.get('[data-testid="remote-disconnect"]').trigger('click')
  expect(useRemoteConnection().endpoint.value).toBeNull()
  reopened.unmount()
})

vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/remoteservice', () => ({
  Connect: vi.fn(() => Promise.resolve('remote-one')),
  Disconnect: vi.fn(async () => {}),
}))

it('Connect saves the profile even if the remote is unreachable', async () => {
  mocks.list.mockRejectedValue(new Error('Remote unavailable'))
  const wrapper = mount(RemoteSettingsView)
  await wrapper.get('[data-testid="remote-name"]').setValue('Saved offline')
  await wrapper.get('[data-testid="remote-host"]').setValue('offline-server')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(useRemoteConnection().profiles.value.some((row) => row.name === 'Saved offline')).toBe(true)
  await nextTick()
  wrapper.unmount()
  resetStores()
  expect(useRemoteConnection().profiles.value.some((row) => row.name === 'Saved offline')).toBe(true)
})
