import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { useConfirmation } from '../../composables/useConfirmation'

const mocks = vi.hoisted(() => ({
  Status: vi.fn(),
  InstallUpdate: vi.fn(),
  handlers: new Map<string, (event: { data: unknown }) => void>(),
}))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/updaterservice', () => ({
  Status: mocks.Status,
  InstallUpdate: mocks.InstallUpdate,
}))
vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, handler: (event: { data: unknown }) => void) => {
      mocks.handlers.set(name, handler)
      return () => mocks.handlers.delete(name)
    },
  },
}))

import { useSelfUpdate } from '../useSelfUpdate'

const showToast = vi.fn()

function setup() {
  const confirmation = useConfirmation()
  let update!: ReturnType<typeof useSelfUpdate>
  mount(
    defineComponent({
      setup() {
        update = useSelfUpdate(confirmation, showToast)
        return () => null
      },
    }),
  )
  return { confirmation, update }
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.Status.mockResolvedValue({ available: false, latestVersion: '' })
})

describe('useSelfUpdate', () => {
  it('seeds the chip from Status and follows update:available', async () => {
    mocks.Status.mockResolvedValue({ available: true, latestVersion: '1.2.0' })
    const { update } = setup()
    await flushPromises()
    expect(update.available.value).toBe(true)
    expect(update.latestVersion.value).toBe('1.2.0')

    mocks.handlers.get('update:available')?.({ data: [{ available: true, latestVersion: '1.3.0' }] })
    expect(update.latestVersion.value).toBe('1.3.0')
  })

  it('installs only after confirmation', () => {
    mocks.InstallUpdate.mockReturnValue(new Promise(() => {}))
    const { confirmation, update } = setup()

    update.openUpdate()
    expect(confirmation.options.value?.testid).toBe('update-confirmation')
    expect(mocks.InstallUpdate).not.toHaveBeenCalled()

    void confirmation.confirm()
    expect(mocks.InstallUpdate).toHaveBeenCalledOnce()
    expect(update.installing.value).toBe(true)

    update.openUpdate()
    expect(confirmation.busy.value).toBe(true)
  })

  it('reports a failed install and lets the user try again', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.InstallUpdate.mockRejectedValue(new Error('disk full'))
    const { confirmation, update } = setup()

    update.openUpdate()
    await confirmation.confirm()

    expect(update.installing.value).toBe(false)
    expect(confirmation.error.value).toBe('disk full')
    expect(showToast).toHaveBeenCalledWith(
      'Could not install the update',
      expect.objectContaining({ severity: 'error', body: 'disk full' }),
    )
  })
})
