import { config, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach, vi } from 'vitest'
import { installHiveUi } from './platform'
import { fakePlatform } from './test-utils/platform'

config.global.plugins = [[installHiveUi, fakePlatform]]

// Leaked wrappers stay attached to the document, and each carries its own Vue
// app whose `useId()` counter restarts at the same value, so a stale wrapper's
// `id` shadows the live one.
enableAutoUnmount(afterEach)

// Node (22+) ships its own global `localStorage` that shadows happy-dom's and
// throws without a `--localstorage-file` backing path. This in-memory stand-in
// is installed once at load, and beforeEach clears the same instance so
// references a module captured earlier stay valid.
function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key: string) => values.get(key) ?? null,
    key: (index: number) => [...values.keys()][index] ?? null,
    removeItem: (key: string) => values.delete(key),
    setItem: (key: string, value: string) => values.set(key, value),
  }
}

Object.defineProperty(globalThis, 'localStorage', {
  value: memoryStorage(),
  writable: true,
  configurable: true,
})

beforeEach(() => {
  localStorage.clear()
  vi.mocked(fakePlatform.clipboard.writeText).mockReset().mockResolvedValue(undefined)
  vi.mocked(fakePlatform.toast).mockReset()
  fakePlatform.theme.value = 'dark'
})
