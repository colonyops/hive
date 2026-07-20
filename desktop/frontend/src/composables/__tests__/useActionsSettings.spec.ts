import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'

const mocks = vi.hoisted(() => ({
  ListActions: vi.fn(), CreateAction: vi.fn(), UpdateAction: vi.fn(), DeleteAction: vi.fn(), On: vi.fn(),
}))

vi.mock('../../../bindings/github.com/colonyops/hive/desktop/actionsservice', () => ({
  ListActions: mocks.ListActions, CreateAction: mocks.CreateAction, UpdateAction: mocks.UpdateAction, DeleteAction: mocks.DeleteAction,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

import { useActionsSettings } from '../useActionsSettings'

const oldCatalog = { actions: [{ id: 'old', label: 'Old', type: 'shell', showInDetail: true, appliesTo: [], shell: { commandTemplate: 'true' } }], error: '' }
const newCatalog = { actions: [{ id: 'new', label: 'New', type: 'launch-session', showInDetail: true, appliesTo: [], launch: { promptTemplate: 'go' } }], error: '' }

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

function mountHarness() {
  let state!: ReturnType<typeof useActionsSettings>
  const Host = defineComponent({ setup: () => { state = useActionsSettings(); return () => null } })
  return { wrapper: mount(Host), state: () => state }
}

afterEach(() => vi.clearAllMocks())

describe('useActionsSettings', () => {
  it('coalesces duplicate wakes, serializes reads, and prevents stale responses from winning', async () => {
    const first = deferred<typeof oldCatalog>()
    const second = deferred<typeof newCatalog>()
    mocks.ListActions.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    let wake!: () => void
    mocks.On.mockImplementation((_topic: string, callback: () => void) => { wake = callback; return () => {} })
    const { wrapper, state } = mountHarness()
    await nextTick()

    wake()
    wake()
    expect(mocks.ListActions).toHaveBeenCalledTimes(1)
    first.resolve(oldCatalog)
    await flushPromises()
    expect(mocks.ListActions).toHaveBeenCalledTimes(2)
    expect(state().actions.value).toEqual([])

    second.resolve(newCatalog)
    await flushPromises()
    expect(state().actions.value).toEqual(newCatalog.actions)
    wrapper.unmount()
  })

  it('passes typed create/update/delete DTOs and reloads after successful writes', async () => {
    mocks.ListActions.mockResolvedValue({ actions: [], error: '' })
    mocks.CreateAction.mockResolvedValue(newCatalog.actions[0])
    mocks.UpdateAction.mockResolvedValue({ ...newCatalog.actions[0], label: 'Changed' })
    mocks.DeleteAction.mockResolvedValue(undefined)
    mocks.On.mockReturnValue(() => {})
    const { wrapper, state } = mountHarness()
    await flushPromises()
    const action = newCatalog.actions[0]

    await state().create(action)
    await state().update('new', { ...action, label: 'Changed' })
    await state().remove('new')
    expect(mocks.CreateAction).toHaveBeenCalledWith(action)
    expect(mocks.UpdateAction).toHaveBeenCalledWith('new', { ...action, label: 'Changed' })
    expect(mocks.DeleteAction).toHaveBeenCalledWith('new')
    expect(mocks.ListActions).toHaveBeenCalledTimes(4)
    wrapper.unmount()
  })

  it('retains last-good actions while surfacing catalog and mutation errors', async () => {
    mocks.ListActions.mockResolvedValue({ actions: oldCatalog.actions, error: 'actions: malformed YAML' })
    mocks.CreateAction.mockRejectedValue(new Error('duplicate action'))
    mocks.UpdateAction.mockRejectedValue(new Error('invalid timeout'))
    mocks.On.mockReturnValue(() => {})
    const { wrapper, state } = mountHarness()
    await flushPromises()
    expect(state().actions.value).toEqual(oldCatalog.actions)
    expect(state().error.value).toContain('malformed YAML')

    await state().create(oldCatalog.actions[0])
    expect(state().actions.value).toEqual(oldCatalog.actions)
    expect(state().error.value).toBe('duplicate action')
    await state().update('old', oldCatalog.actions[0])
    expect(state().error.value).toBe('invalid timeout')
    wrapper.unmount()
  })
})
