import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { usePersistedSetting } from '../usePersistedSetting'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

describe('usePersistedSetting', () => {
  beforeEach(() => {
    vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => vi.restoreAllMocks())

  it('holds the initial value until the stored one arrives', async () => {
    const read = deferred<number>()
    const setting = usePersistedSetting({ initial: 3, read: () => read.promise, write: vi.fn(), label: 'the size' })

    expect(setting.value.value).toBe(3)
    expect(setting.hydrated.value).toBe(false)

    read.resolve(5)
    await setting.whenHydrated()

    expect(setting.value.value).toBe(5)
    expect(setting.hydrated.value).toBe(true)
  })

  it('keeps the initial value when the file holds none', async () => {
    const setting = usePersistedSetting<number>({
      initial: 3,
      read: () => Promise.resolve(undefined),
      write: vi.fn(),
      label: 'the size',
    })

    await setting.whenHydrated()

    expect(setting.value.value).toBe(3)
    expect(setting.hydrated.value).toBe(true)
  })

  it('warns and keeps the initial value when the read fails', async () => {
    const setting = usePersistedSetting<number>({
      initial: 3,
      read: () => Promise.reject(new Error('no file')),
      write: vi.fn(),
      label: 'the size',
    })

    await setting.whenHydrated()

    expect(setting.value.value).toBe(3)
    expect(setting.hydrated.value).toBe(true)
    expect(console.warn).toHaveBeenCalledWith('Unable to load the size from settings.yaml', expect.any(Error))
  })

  it('applies a write at once and persists it', async () => {
    const write = vi.fn().mockResolvedValue(undefined)
    const setting = usePersistedSetting({ initial: 3, read: () => Promise.resolve(3), write, label: 'the size' })
    await setting.whenHydrated()

    setting.set(4)

    expect(setting.value.value).toBe(4)
    await flushPromises()
    expect(write).toHaveBeenCalledWith(4)
  })

  it('lets a write made during the read win over the read result', async () => {
    const read = deferred<number>()
    const setting = usePersistedSetting({
      initial: 3,
      read: () => read.promise,
      write: vi.fn().mockResolvedValue(undefined),
      label: 'the size',
    })

    setting.set(6)
    read.resolve(2)
    await setting.whenHydrated()

    expect(setting.value.value).toBe(6)
  })

  it('persists writes in order and carries on after one fails', async () => {
    const first = deferred<void>()
    const write = vi.fn().mockReturnValueOnce(first.promise).mockResolvedValue(undefined)
    const setting = usePersistedSetting({ initial: 1, read: () => Promise.resolve(1), write, label: 'the size' })
    await setting.whenHydrated()

    setting.set(2)
    setting.set(3)
    await flushPromises()
    expect(write).toHaveBeenCalledTimes(1)

    first.reject(new Error('disk full'))
    await flushPromises()

    expect(write).toHaveBeenCalledTimes(2)
    expect(write).toHaveBeenLastCalledWith(3)
    expect(setting.value.value).toBe(3)
    expect(console.warn).toHaveBeenCalledWith('Unable to persist the size to settings.yaml', expect.any(Error))
  })
})
