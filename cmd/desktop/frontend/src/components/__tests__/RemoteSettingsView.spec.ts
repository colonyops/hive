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
  await wrapper.get('[data-testid="remote-color"]').setValue('#22c55e')
  await wrapper.get('[data-testid="remote-token"]').setValue('private-token')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(mocks.push).toHaveBeenCalledWith({ name: 'terminal' })
  expect(useCodeInstallation().installation.value).toBe('remote')
  expect((wrapper.get('[data-testid="remote-token"]').element as HTMLInputElement).value).toBe('')
  wrapper.unmount()
  expect(useRemoteConnection().endpoint.value?.token).toBe('private-token')
  const reopened = mount(RemoteSettingsView)
  expect((reopened.get('[data-testid="remote-name"]').element as HTMLInputElement).value).toBe('Build machine')
  expect((reopened.get('[data-testid="remote-color"]').element as HTMLInputElement).value).toBe('#22c55e')
  await reopened.get('[data-testid="remote-disconnect"]').trigger('click')
  expect(useRemoteConnection().endpoint.value).toBeNull()
  reopened.unmount()
})
