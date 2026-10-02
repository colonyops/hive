import { effectScope, type EffectScope } from 'vue'

interface StoreHandle {
  stop(): void
}

const started = new Set<StoreHandle>()

/**
 * Declares a shared-state store. `setup` runs once, on first use, inside a
 * detached effect scope the store owns: a `watch`, `computed`, or
 * `useWailsEvent` created there lives until `resetStores()` stops it, not
 * until the first component that happened to call the store unmounts.
 *
 * A store must not call `inject`, `onMounted`, or any other hook that binds
 * to the current component. The scope is detached; the component context is
 * not.
 */
export function defineStore<T extends object>(name: string, setup: () => T): () => T {
  let current: { scope: EffectScope; instance: T } | undefined
  let settingUp = false

  const handle: StoreHandle = {
    stop() {
      current?.scope.stop()
      current = undefined
    },
  }

  return function useStore(): T {
    if (current) return current.instance
    if (settingUp) throw new Error(`store "${name}" uses itself during setup`)

    settingUp = true
    const scope = effectScope(true)
    try {
      const instance = scope.run(setup)
      if (instance === undefined) throw new Error(`store "${name}" setup returned nothing`)
      current = { scope, instance }
    } catch (error) {
      scope.stop()
      throw error
    } finally {
      settingUp = false
    }
    started.add(handle)
    return current.instance
  }
}

/**
 * Stops every started store's scope and forgets its instance, so the next use
 * runs `setup` again. The test setup calls this after every test.
 */
export function resetStores(): void {
  for (const store of started) store.stop()
  started.clear()
}
