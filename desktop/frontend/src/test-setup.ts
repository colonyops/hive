import { beforeEach } from 'vitest'

// Node (22+) ships its own global `localStorage`/`sessionStorage`, which
// shadows happy-dom's Storage implementation in this Vitest environment and
// throws ("getItem is not a function") without a `--localstorage-file`
// backing path. Every real runtime target (a browser, the Wails webview) has a
// working localStorage, so this in-memory stand-in exists purely so tests
// exercise the same code paths (useTheme, useResizablePanel, ...) as
// production.
//
// It is installed ONCE, at setup-module load — before any test file (and its
// module-singleton `useStorage` calls, e.g. useTheme) evaluates — so those
// capture a working, stable Storage reference. beforeEach then `clear()`s that
// same instance (rather than swapping in a new one) to keep specs isolated
// without invalidating references already captured by singletons.
function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() { return values.size },
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
})
