import { CompressionStream, DecompressionStream } from 'node:stream/web'
import { installHiveUi } from '@hive/ui/platform'
import { config, enableAutoUnmount } from '@vue/test-utils'
import { afterAll, afterEach, beforeEach, vi } from 'vitest'
import { resetStores } from './stores/defineStore'
import { installMemoryStorage } from '@hive/ui/test-utils/memoryStorage'

// Registered before enableAutoUnmount so that, with Vitest's stack ordering of
// after-hooks, every mounted wrapper is gone before the stores it used stop.
afterEach(resetStores)

// Keeps every spec that logs off the network.
vi.mock('../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/clientlogservice', () => ({
  Level: vi.fn(() => Promise.resolve('debug')),
  Write: vi.fn(() => Promise.resolve()),
}))

// usePerf flushes on a timer, so a component that records a sample can reach
// the binding mid-spec, where a spec's own @wailsio/runtime mock has no Call.
vi.mock('../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/perfservice', () => ({
  Info: vi.fn(() => Promise.resolve({ enabled: false })),
  Record: vi.fn(() => Promise.resolve()),
}))

// The panel layout stores report every change to Go, so any spec that opens a
// tab would otherwise reach the binding.
vi.mock('../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/layoutservice', () => ({
  Report: vi.fn(() => Promise.resolve()),
}))

// Stores and composables subscribe as they mount, and most specs do not care.
// A spec that asserts on events mocks @wailsio/runtime itself, which replaces this.
vi.mock('@wailsio/runtime', async (importOriginal) => {
  const runtime = await importOriginal<typeof import('@wailsio/runtime')>()
  return { ...runtime, Events: { ...runtime.Events, On: vi.fn(() => () => {}) } }
})

// Every `mount` is torn down between tests. Leaked wrappers stay attached to
// the document, and each carries its own Vue app whose `useId()` counter
// restarts at the same value — so a stale wrapper's `id` shadows the live one
// and `form=`/`aria-controls` lookups resolve to the wrong element.
enableAutoUnmount(afterEach)

// The vmThreads pool runs each spec in its own vm context, and a few globals
// still come from the outer one: TextEncoder returns an outer-realm Uint8Array,
// which toEqual will not match against a Uint8Array a spec builds, and the
// compression streams are not exposed in the context at all.
const OuterTextEncoder = globalThis.TextEncoder
globalThis.TextEncoder = class extends OuterTextEncoder {
  override encode(input?: string): Uint8Array<ArrayBuffer> {
    return new Uint8Array(super.encode(input))
  }
}
globalThis.CompressionStream ??= CompressionStream as unknown as typeof globalThis.CompressionStream
globalThis.DecompressionStream ??= DecompressionStream as unknown as typeof globalThis.DecompressionStream

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
installMemoryStorage()

// Unit tests must never open real sockets. happy-dom's default origin is
// http://localhost:3000, so an unmocked Wails binding call (@wailsio/runtime
// POSTs to <origin>/wails/runtime via fetch) otherwise dials a real port —
// and a refused connection can surface as an uncaught AggregateError outside
// any promise chain, failing a CI run whose tests all passed. A synthetic 503
// keeps the callers on the same rejected-promise path they already handle,
// with no socket involved. Specs that need a different fetch can still stub
// their own (the property stays writable).
Object.defineProperty(globalThis, 'fetch', {
  value: () =>
    Promise.resolve(
      new Response('{"error":"network disabled in unit tests"}', {
        status: 503,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  writable: true,
  configurable: true,
})

// Loaded per test, not at setup time: a spec's vi.mock('@wailsio/runtime')
// registers after this file runs, and an import made now would keep the real
// Clipboard.
beforeEach(async () => {
  localStorage.clear()
  const { hiveUiPlatform } = await import('./lib/hiveUiPlatform')
  config.global.plugins = [[installHiveUi, hiveUiPlatform]]
})
