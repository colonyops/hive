# Stores

Shared frontend state lives here: one store per file, each declared with
`defineStore`. A store is the single owner of a piece of state that outlives
any one component. Components and composables read it and call its actions;
nothing else writes it.

## Store or composable?

A **store** holds state that more than one place reads and that outlives the
component that first touched it: the active jobs list, a setting, whether the
window is focused, the result of an availability probe. It lives in
`src/stores/` and is created with `defineStore`.

A **composable** holds no shared state. It is either per-instance, created
fresh for each caller (`useTerminalWindows`, `useResizablePanel`), or
stateless glue over the Vue lifecycle (`useWailsEvent`, `useEscapeToClose`).
It lives in `src/composables/`.

The lint enforces the split. A module-level `ref`, `shallowRef`, `reactive`,
`shallowReactive`, or `useStorage` outside `src/stores/` is an error. Inside
`src/stores/` the same call is an error unless it sits inside a `defineStore`
setup. A raw `Events.On` anywhere but `useWailsEvent` is an error. Violations
that predate the rules are recorded in `eslint-suppressions.json`; each
migration prunes its file's entries (see `cmd/desktop/AGENTS.md`).

## Writing a store

```ts
// src/stores/useJobs.ts
import { computed, readonly } from 'vue'
import { ListActive } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/jobservice'
import type { Job } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/jobs/models'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

export const useJobs = defineStore('jobs', () => {
  const jobs = useResource(async () => (await ListActive()) ?? [], {
    initial: [] as Job[],
    errorFallback: 'Could not load active jobs.',
  })
  const hasActive = computed(() => jobs.data.value.length > 0)

  useWailsEvent('jobs:updated', () => {
    void jobs.reload()
  })
  void jobs.reload()

  return { activeJobs: readonly(jobs.data), hasActive, error: jobs.error, reload: jobs.reload }
})
```

The shipped `useJobs.ts` is this plus a trailing re-read on a timer, cleared
in `onScopeDispose`; it is the reference for a complete store.

`defineStore(name, setup)` runs `setup` once, on first use, inside a detached
`effectScope` the store owns. A `watch`, `computed`, or `useWailsEvent`
created there is torn down by `resetStores()`, not by whichever component
happened to call the store first. Every later call returns the same object.

A store starts itself. The first load is the `void reload()` at the end of
setup. There is no `start()`, no `started` flag, and no component calling
`reload` from `onMounted` to get the initial data.

Inside setup:

- Do not call `inject`, `provide`, `onMounted`, or any other hook that binds
  to the current component. The scope is detached; the component context is
  not, so the hook would attach to an arbitrary component.
- A store may use another store. Call it inside setup like anywhere else.
- Subscribe to Wails events with `useWailsEvent`, never `Events.On`. The
  subscription ends when the store scope stops.
- Anything else the store starts (a timer, an `AbortController`) is stopped
  in `onScopeDispose`.

The `name` is the file's noun (`'jobs'` for `useJobs.ts`). It only appears in
error messages.

## Building blocks

### A list, or anything fetched from a service: `useResource`

```ts
const rows = useResource(fetch, { initial, errorFallback })
// rows.data     ShallowRef<T>            replaced whole by each successful load
// rows.loading  Readonly<Ref<boolean>>   a request is running
// rows.loaded   Readonly<Ref<boolean>>   the first request has settled, success or not
// rows.error    Readonly<Ref<string | null>>
// rows.reload() Promise<void>            never rejects; a failure lands in error
```

`reload` runs one request at a time. A call that arrives while a request is
running queues one follow-up after it, and every caller's promise settles
after a request that started no earlier than its call. So a wake-up event
that lands mid-request still produces a fresh read, and three events in a
burst cost two requests, not three. A failed load keeps the last good `data`.

`data` is a `shallowRef`: replace it, never mutate it in place. An action that
changes one row does `rows.data.value = rows.data.value.map(...)`.

### A probe

An availability probe is a resource whose data is the answer:

```ts
const probe = useResource(() => Available(), {
  initial: { available: false, reason: '' },
  errorFallback: 'The pop-up terminal is unavailable.',
})
void probe.reload()
return {
  checking: computed(() => !probe.loaded.value),
  available: computed(() => probe.data.value.available),
  reason: computed(() => probe.error.value ?? probe.data.value.reason),
  recheck: probe.reload,
}
```

### A setting

A persisted setting hydrates once on first use, applies a write to the ref
immediately, persists writes in the order they were made, and warns on a
failed persist. `usePersistedSetting(read, write)` will own that pattern and
arrives with the settings migration (colonyops/hive#536, step 3). Until then a
settings store follows `useTerminalFont`'s shape inside a `defineStore` setup.

### Error text

`errorText(err, fallback)` in `src/lib/appError.ts` is the one way to turn a
caught error into a string: the Go core's message when the call reached Go,
the thrown message when it did not, `fallback` when neither says anything.
Do not write another `err instanceof Error ? err.message : ...`.

## Conventions

- A store returns **readonly state and named actions**. Expose a resource's
  data as `readonly(rows.data)`; `loading`, `loaded`, and `error` are already
  readonly. A consumer never assigns to store state; it calls an action. When
  a consumer's prop or parameter is typed with a mutable array, change it to
  `readonly T[]`.
- The reload action is always called **`reload`**. Not `refresh`, `load`, or
  `reloadThings`.
- An error is always **`string | null`**, with `null` meaning no error. Not
  `''`, not `unknown`, not a toast from inside the store. A store whose error
  no surface renders logs it with `console.warn` so it is not lost.
- No `reset…ForTests` helpers. `resetStores()` covers every store.
- One store per file, the file named after the hook it exports.

## Testing

`resetStores()` runs after every test (see `src/test-setup.ts`), so each test
starts with no store started and the first use runs setup again. That replaces
`vi.resetModules()` and dynamic imports: import the store at the top of the
spec.

```ts
import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetStores } from '../defineStore'
import { useJobs } from '../useJobs'

const mocks = vi.hoisted(() => ({ ListActive: vi.fn(), On: vi.fn() }))
vi.mock('../../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/jobservice', () => ({
  ListActive: mocks.ListActive,
}))
vi.mock('@wailsio/runtime', () => ({ Events: { On: mocks.On } }))

describe('useJobs', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.On.mockReturnValue(() => {})
    mocks.ListActive.mockResolvedValue([])
  })

  it('reloads on jobs:updated', async () => {
    const jobs = useJobs()
    await flushPromises()
    mocks.ListActive.mockResolvedValue([{ id: 1, status: 'running' }])
    mocks.On.mock.calls[0][1]()
    await vi.waitFor(() => expect(jobs.hasActive.value).toBe(true))
  })
})
```

- Mock `Events.On` to **return a function**. The store scope calls it as the
  unsubscribe on reset. `useWailsEvent` throws at subscription time when the
  mock returns nothing, so a bare `vi.fn()` fails the first test that starts
  the store. A store that a module calls at import time starts before any
  `beforeEach`, so set the return value in `vi.hoisted`:
  `On: vi.fn().mockReturnValue(() => {})`.
- Mock the binding module the store fetches from, not the store.
- To start over mid-test, call `resetStores()` yourself.
- A component spec that mounts something using a store needs no setup beyond
  the mocks above; the store starts on the component's first call.

## Migrating a composable

1. `git mv src/composables/useThing.ts src/stores/useThing.ts` and wrap the
   body in `defineStore('thing', () => { ... })`. Module-level refs move
   inside.
2. Replace the hand-rolled list state with `useResource`. Rename the reload
   function to `reload`. Make errors `string | null` through `errorText`.
3. Delete the `start()` / `started` pair; the end of setup starts the store.
   Replace `Events.On` with `useWailsEvent`.
4. Return readonly state. Fix any consumer that wrote the old refs directly
   by giving the store an action.
5. Delete `resetThingForTests` and its callers. Move the spec next to the
   store and drop `vi.resetModules()`.
6. Update imports, then run `npx eslint --prune-suppressions .` in
   `cmd/desktop/frontend/` so the file's old entries leave
   `eslint-suppressions.json`.
