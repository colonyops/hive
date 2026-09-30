import { enableAutoUnmount } from '@vue/test-utils'
import { afterAll, afterEach, beforeEach } from 'vitest'

// Every `mount` is torn down between tests. Leaked wrappers stay attached to
// the document, and each carries its own Vue app whose `useId()` counter
// restarts at the same value — so a stale wrapper's `id` shadows the live one
// and `form=`/`aria-controls` lookups resolve to the wrong element.
enableAutoUnmount(afterEach)

// @wailsio/runtime's drag module starts a 50ms polling interval as an
// import-time side effect, and that interval's very first tick dereferences
// `window` (it calls window.clearInterval to stop itself). Any spec importing
// the Wails runtime — which includes anything reaching a generated service
// binding — that finishes within those 50ms leaves the timer pending, so it
// fires after Vitest has torn the happy-dom environment down and raises
// `ReferenceError: window is not defined`. That surfaces as an unhandled error
// which fails the whole run regardless of how the assertions went, and it is
// timing-dependent: fast local runs usually win the race, slower CI ones do
// not.
//
// setupFiles run before any test module is evaluated, and `window` is
// `globalThis` under this environment, so patching here catches the runtime's
// `window.setInterval` call and lets afterAll clear it while `window` is still
// alive. Intervals only: this is about a module-scope poller outliving its
// environment, not about test-owned timers.
const pendingIntervals = new Set<ReturnType<typeof setInterval>>()
const nativeSetInterval = globalThis.setInterval
globalThis.setInterval = ((...args: Parameters<typeof setInterval>) => {
  const id = nativeSetInterval(...args)
  pendingIntervals.add(id)
  return id
}) as typeof globalThis.setInterval

afterAll(() => {
  for (const id of pendingIntervals) clearInterval(id)
  pendingIntervals.clear()
})

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

// Unit tests must never open real sockets. happy-dom's default origin is
// http://localhost:3000, so an unmocked Wails binding call (@wailsio/runtime
// POSTs to <origin>/wails/runtime via fetch) otherwise dials a real port —
// and a refused connection can surface as an uncaught AggregateError outside
// any promise chain, failing a CI run whose tests all passed. A synthetic 503
// keeps the callers on the same rejected-promise path they already handle,
// with no socket involved. Specs that need a different fetch can still stub
// their own (the property stays writable).
Object.defineProperty(globalThis, 'fetch', {
  value: () => Promise.resolve(new Response('{"error":"network disabled in unit tests"}', {
    status: 503,
    headers: { 'Content-Type': 'application/json' },
  })),
  writable: true,
  configurable: true,
})

beforeEach(() => {
  localStorage.clear()
})
