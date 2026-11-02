// Node (22+) ships its own global `localStorage` that shadows happy-dom's and
// throws without a `--localstorage-file` backing path. A setup file installs this
// once at load, before any module-level `useStorage` call captures the reference,
// and clears that same instance between tests.
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

export function installMemoryStorage() {
  Object.defineProperty(globalThis, 'localStorage', {
    value: memoryStorage(),
    writable: true,
    configurable: true,
  })
}
