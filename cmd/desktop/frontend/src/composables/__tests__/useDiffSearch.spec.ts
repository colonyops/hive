import { describe, expect, it, vi } from 'vitest'
import { nextTick, ref, shallowRef } from 'vue'
import { useDiffSearch } from '../useDiffSearch'
import type { DiffEntry } from '../useFileDiffs'
import type { ChangedFile } from '../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

vi.mock('@vueuse/core', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@vueuse/core')>()),
  refDebounced: <T>(source: T) => source,
}))

const entry = (text: string) => ({
  version: '1',
  diff: { path: '', state: 'text', text, error: '' },
})
const hit = entry('@@ -1 +1 @@\n-old\n+needle\n')
const file = (path: string) => ({ path }) as ChangedFile

function setup() {
  const reveal = vi.fn()
  const entries = shallowRef<ReadonlyMap<string, DiffEntry>>(new Map())
  const search = useDiffSearch(ref([file('a.go'), file('b.go')]), entries, reveal)
  return { reveal, entries, search }
}

describe('useDiffSearch', () => {
  it('reveals the first match when its diff is read after the query landed', async () => {
    const { reveal, entries, search } = setup()
    search.query.value = 'needle'
    await nextTick()
    expect(reveal).not.toHaveBeenCalled()

    entries.value = new Map([['a.go', hit]])
    await nextTick()
    expect(reveal).toHaveBeenCalledTimes(1)
    expect(search.activeIndex.value).toBe(0)
  })

  it('stays on the current match as more diffs arrive', async () => {
    const { reveal, entries, search } = setup()
    entries.value = new Map([['b.go', hit]])
    search.query.value = 'needle'
    await nextTick()
    reveal.mockClear()

    entries.value = new Map([
      ['a.go', hit],
      ['b.go', hit],
    ])
    await nextTick()
    expect(reveal).not.toHaveBeenCalled()
    expect(search.active.value?.path).toBe('b.go')
    expect(search.activeIndex.value).toBe(1)
  })

  it('does not scroll when a refresh removes the current match', async () => {
    const { reveal, entries, search } = setup()
    entries.value = new Map([
      ['a.go', hit],
      ['b.go', hit],
    ])
    search.query.value = 'needle'
    await nextTick()
    search.step(1)
    expect(search.active.value?.path).toBe('b.go')
    reveal.mockClear()

    entries.value = new Map([['a.go', hit]])
    await nextTick()
    expect(reveal).not.toHaveBeenCalled()
    expect(search.active.value?.path).toBe('a.go')
  })

  it('wraps around in both directions', async () => {
    const { entries, search } = setup()
    entries.value = new Map([
      ['a.go', hit],
      ['b.go', hit],
    ])
    search.query.value = 'needle'
    await nextTick()
    search.step(-1)
    expect(search.active.value?.path).toBe('b.go')
    search.step(1)
    expect(search.active.value?.path).toBe('a.go')
  })
})
