import { beforeEach } from 'vitest'

// Node (22+) ships its own global `localStorage`/`sessionStorage`, which
// shadows happy-dom's Storage implementation in this Vitest environment and
// throws ("getItem is not a function") without a `--localstorage-file`
// backing path. Every real runtime target (a browser, the Wails webview)
// has a working localStorage, so this in-memory stand-in exists purely so
// tests exercise the same code paths (useTheme, useResizablePanel, ...) as
// production. A fresh instance per test keeps specs isolated; individual
// specs may still layer their own `vi.stubGlobal('localStorage', ...)` on
// top (see SettingsView.spec.ts) — `vi.unstubAllGlobals()` there simply
// reverts to whatever this hook set for that test.
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

beforeEach(() => {
  Object.defineProperty(globalThis, 'localStorage', {
    value: memoryStorage(),
    writable: true,
    configurable: true,
  })
})
