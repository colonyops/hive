import { readonly, ref, type Ref } from 'vue'

export interface PersistedSettingOptions<T> {
  /** The value until the stored one has been read. */
  initial: T
  /** Reads the stored value. `undefined` keeps the current value, for a file that holds none. */
  read: () => Promise<T | undefined>
  write: (value: T) => Promise<void>
  /** Names the setting in the two warnings, as in "the terminal pool size". */
  label: string
}

export interface PersistedSetting<T> {
  value: Readonly<Ref<T>>
  /** True once the stored value has been read, or the read has failed. */
  hydrated: Readonly<Ref<boolean>>
  /** Applies `next` at once and persists it after every earlier write of this setting. */
  set: (next: T) => void
  /** Resolves once the stored value has been read; for an action relative to it. */
  whenHydrated: () => Promise<void>
}

/**
 * The settings pattern: hydrate once from settings.yaml, apply a write
 * immediately, persist writes in order, and warn when either side fails.
 * settings.yaml stays the only store; a failed persist keeps the chosen value
 * on screen so the control never snaps back under the user's hand.
 */
export function usePersistedSetting<T>(options: PersistedSettingOptions<T>): PersistedSetting<T> {
  const value = ref(options.initial) as Ref<T>
  const hydrated = ref(false)
  let version = 0
  let persistChain: Promise<void> = Promise.resolve()

  // A write made while the read is in flight must win over the read's result.
  const hydration = (async () => {
    const startedAt = version
    try {
      const stored = await options.read()
      if (version === startedAt && stored !== undefined) value.value = stored
    } catch (error) {
      console.warn(`Unable to load ${options.label} from settings.yaml`, error)
    } finally {
      hydrated.value = true
    }
  })()

  function set(next: T): void {
    version++
    value.value = next
    persistChain = persistChain
      .then(() => options.write(next))
      .catch((error: unknown) => {
        console.warn(`Unable to persist ${options.label} to settings.yaml`, error)
      })
  }

  return {
    value: readonly(value) as Readonly<Ref<T>>,
    hydrated: readonly(hydrated),
    set,
    whenHydrated: () => hydration,
  }
}
