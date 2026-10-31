import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import { fileVersion, useFileDiffs } from '../useFileDiffs'
import type { DiffBase } from '../useSessionChanges'

const mocks = vi.hoisted(() => ({ SessionFileDiffs: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice', () => ({
  SessionFileDiffs: mocks.SessionFileDiffs,
}))

function file(path: string, stamp = '1') {
  return { path, oldPath: '', status: 'modified', additions: 1, deletions: 0, binary: false, stamp }
}
function diff(path: string, text = `@@ ${path}`) {
  return { path, text, binary: false, truncated: false, unavailable: false, gone: false, error: '' }
}

function setup(paths = ['a', 'b']) {
  const sessionId = ref('1')
  const base = ref<DiffBase>('branch')
  const files = ref(paths.map((path) => file(path)))
  const wanted = ref<string[]>(paths)
  const target = computed(() => ({ base: base.value, against: '', commit: '' }))
  const api = useFileDiffs(sessionId, target, files, wanted)
  return { ...api, sessionId, base, files, wanted }
}

describe('useFileDiffs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.SessionFileDiffs.mockImplementation((_id: string, _target: unknown, paths: string[]) =>
      Promise.resolve(paths.map((path) => diff(path))),
    )
  })

  it('reads the wanted files in one batch', async () => {
    const { entries } = setup()
    await flushPromises()
    expect(mocks.SessionFileDiffs).toHaveBeenCalledTimes(1)
    expect(mocks.SessionFileDiffs).toHaveBeenCalledWith('1', { base: 'branch', against: '', commit: '' }, ['a', 'b'])
    expect(entries.value.get('a')?.diff.text).toBe('@@ a')
  })

  it('reads a long list in batches, one request at a time', async () => {
    let running = 0
    let peak = 0
    mocks.SessionFileDiffs.mockImplementation(async (_id: string, _target: unknown, paths: string[]) => {
      peak = Math.max(peak, ++running)
      await Promise.resolve()
      running--
      return paths.map((path) => diff(path))
    })
    const paths = Array.from({ length: 120 }, (_, index) => `f${index}`)
    const { entries } = setup(paths)
    await flushPromises()
    expect(mocks.SessionFileDiffs.mock.calls.map(([, , batch]) => (batch as string[]).length)).toEqual([50, 50, 20])
    expect(peak).toBe(1)
    expect(entries.value.size).toBe(120)
  })

  it('does not read again until a file version moves, and keeps the old diff meanwhile', async () => {
    const { files, entries } = setup(['a'])
    await flushPromises()
    files.value = [file('a')]
    await flushPromises()
    expect(mocks.SessionFileDiffs).toHaveBeenCalledTimes(1)

    let finish!: (value: unknown) => void
    mocks.SessionFileDiffs.mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)))
    files.value = [file('a', '2')]
    await flushPromises()
    expect(mocks.SessionFileDiffs).toHaveBeenCalledTimes(2)
    expect(entries.value.get('a')?.diff.text).toBe('@@ a')

    finish([diff('a', 'fresh')])
    await flushPromises()
    expect(entries.value.get('a')?.diff.text).toBe('fresh')
    expect(entries.value.get('a')?.version).toBe(fileVersion(file('a', '2')))
  })

  it('skips a wanted path that is not in the list', async () => {
    setup(['a'])
    const { wanted } = setup(['a'])
    wanted.value = ['a', 'not-listed']
    await flushPromises()
    for (const [, , batch] of mocks.SessionFileDiffs.mock.calls) expect(batch).not.toContain('not-listed')
  })

  it('drops what it holds when the base changes, and ignores the read in flight', async () => {
    let finish!: (value: unknown) => void
    mocks.SessionFileDiffs.mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)))
    const { base, entries } = setup(['a'])
    await flushPromises()

    base.value = 'uncommitted'
    await flushPromises()
    finish([diff('a', 'stale')])
    await flushPromises()

    expect(mocks.SessionFileDiffs).toHaveBeenLastCalledWith('1', { base: 'uncommitted', against: '', commit: '' }, [
      'a',
    ])
    expect(entries.value.get('a')?.diff.text).toBe('@@ a')
  })

  it('reports a failed request on each file, and retries on the next poll rather than at once', async () => {
    mocks.SessionFileDiffs.mockRejectedValueOnce(new Error('daemon gone'))
    const { files, entries } = setup(['a'])
    await flushPromises()
    expect(entries.value.get('a')?.diff.error).toBe('daemon gone')
    expect(mocks.SessionFileDiffs).toHaveBeenCalledTimes(1)

    files.value = [file('a')]
    await flushPromises()
    expect(mocks.SessionFileDiffs).toHaveBeenCalledTimes(2)
    expect(entries.value.get('a')?.diff.error).toBe('')
  })
})
