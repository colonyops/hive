import { readonly, ref, shallowRef, type Ref, type ShallowRef } from 'vue'
import { errorText } from '../lib/appError'

export interface ResourceOptions<T> {
  /** What `data` holds before the first load. */
  initial: T
  /** What `error` reads when a failure carries no message of its own. */
  errorFallback: string
}

export interface Resource<T> {
  /** Replaced whole by each successful load. A failed load keeps the last good value. */
  data: ShallowRef<T>
  /** True while a request is running, including a queued follow-up. */
  loading: Readonly<Ref<boolean>>
  /** True once the first request has settled, whether or not it succeeded. */
  loaded: Readonly<Ref<boolean>>
  error: Readonly<Ref<string | null>>
  /**
   * Fetches again. Never rejects: a failure lands in `error`. Only one request
   * runs at a time; a call made during one queues a single follow-up after it,
   * so a change that lands mid-request is still read, and every caller's
   * promise settles after a request that started no earlier than its call.
   */
  reload(): Promise<void>
}

/**
 * The reloadable-list pattern: one fetcher, the state a view needs to render
 * it, and a `reload` that coalesces concurrent calls.
 */
export function useResource<T>(fetch: () => Promise<T>, options: ResourceOptions<T>): Resource<T> {
  const data = shallowRef(options.initial)
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  let inFlight: Promise<void> | null = null
  let followUp: Promise<void> | null = null

  async function run(): Promise<void> {
    loading.value = true
    try {
      data.value = await fetch()
      error.value = null
    } catch (err) {
      error.value = errorText(err, options.errorFallback)
    } finally {
      loaded.value = true
      loading.value = followUp !== null
    }
  }

  function reload(): Promise<void> {
    if (inFlight) {
      followUp ??= inFlight.then(() => {
        followUp = null
        return reload()
      })
      return followUp
    }
    inFlight = run().finally(() => {
      inFlight = null
    })
    return inFlight
  }

  return { data, loading: readonly(loading), loaded: readonly(loaded), error: readonly(error), reload }
}
