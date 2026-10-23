import { config, enableAutoUnmount } from '@vue/test-utils'
import { afterEach, beforeEach } from 'vitest'
import { installHiveUi } from '@hive/ui/platform'
import { fakePlatform } from '@hive/ui/test-utils/platform'

config.global.plugins = [[installHiveUi, fakePlatform]]
enableAutoUnmount(afterEach)

// Node (22+) ships its own global `localStorage` that shadows happy-dom's and
// throws without a backing file. This in-memory stand-in is installed once.
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

Object.defineProperty(globalThis, 'localStorage', { value: memoryStorage(), writable: true, configurable: true })

beforeEach(() => {
  localStorage.clear()
})
