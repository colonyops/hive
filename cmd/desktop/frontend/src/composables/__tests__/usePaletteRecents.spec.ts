import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

// recentIds is a module singleton that loads whatever is in localStorage at
// import time (via VueUse useStorage), so a spec that cares about the initial
// read resets modules and imports fresh — the same pattern useTheme.spec.ts
// uses for its own useStorage singleton.
const storageKey = 'hive.palette.recents'

beforeEach(() => {
  localStorage.clear()
  vi.resetModules()
})

describe('usePaletteRecents', () => {
  it('moves a re-run id to the front, dedupes, and caps at RECENT_LIMIT', async () => {
    const { usePaletteRecents, RECENT_LIMIT } = await import('../usePaletteRecents')
    const { recentIds, recordRun } = usePaletteRecents()

    recordRun('a')
    recordRun('b')
    expect(recentIds.value).toEqual(['b', 'a'])

    recordRun('a')
    expect(recentIds.value).toEqual(['a', 'b'])

    for (let i = 1; i <= RECENT_LIMIT + 2; i++) recordRun(`cmd-${i}`)

    expect(recentIds.value).toHaveLength(RECENT_LIMIT)
    expect(recentIds.value).toEqual(
      Array.from({ length: RECENT_LIMIT }, (_, i) => `cmd-${RECENT_LIMIT + 2 - i}`),
    )
  })

  it('round-trips recorded ids through localStorage for a fresh import', async () => {
    const { usePaletteRecents } = await import('../usePaletteRecents')
    usePaletteRecents().recordRun('alpha')
    usePaletteRecents().recordRun('beta')
    await nextTick()

    expect(JSON.parse(localStorage.getItem(storageKey) ?? '[]')).toEqual(['beta', 'alpha'])

    vi.resetModules()
    const { usePaletteRecents: reimported } = await import('../usePaletteRecents')
    expect(reimported().recentIds.value).toEqual(['beta', 'alpha'])
  })

  it('degrades corrupt JSON to an empty list instead of throwing', async () => {
    localStorage.setItem(storageKey, '{not valid json')

    const { usePaletteRecents } = await import('../usePaletteRecents')

    expect(usePaletteRecents().recentIds.value).toEqual([])
  })

  it('degrades well-formed JSON that is not an array of strings to an empty list', async () => {
    localStorage.setItem(storageKey, JSON.stringify({ id: 'not-an-array' }))
    const { usePaletteRecents } = await import('../usePaletteRecents')
    expect(usePaletteRecents().recentIds.value).toEqual([])

    vi.resetModules()
    localStorage.setItem(storageKey, JSON.stringify([1, 2, 3]))
    const { usePaletteRecents: reimported } = await import('../usePaletteRecents')
    expect(reimported().recentIds.value).toEqual([])
  })

  it('truncates a hand-edited array longer than RECENT_LIMIT on read', async () => {
    // Importing just for RECENT_LIMIT still evaluates the module (and its
    // localStorage read), so reset before setting up the real fixture.
    const { RECENT_LIMIT } = await import('../usePaletteRecents')
    vi.resetModules()

    const overLong = Array.from({ length: RECENT_LIMIT + 5 }, (_, i) => `cmd-${i}`)
    localStorage.setItem(storageKey, JSON.stringify(overLong))

    const { usePaletteRecents } = await import('../usePaletteRecents')

    expect(usePaletteRecents().recentIds.value).toEqual(overLong.slice(0, RECENT_LIMIT))
  })

  it('degrades to an empty list when the storage accessor throws, without raising', async () => {
    const getItem = vi.spyOn(localStorage, 'getItem').mockImplementation(() => {
      throw new Error('storage unavailable')
    })
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    try {
      const { usePaletteRecents } = await import('../usePaletteRecents')
      expect(usePaletteRecents().recentIds.value).toEqual([])
      expect(warn).toHaveBeenCalled()
    } finally {
      getItem.mockRestore()
      warn.mockRestore()
    }
  })
})
