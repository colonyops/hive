import { readonly, ref } from 'vue'
import {
  Available,
  Kill,
  Start,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/nativeterminalservice'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'

// Whether this build and machine can run native terminal tabs: a build that
// links libghostty, and herdr-core installed. The answer is the backend
// process's, so it is probed once.
export const useNativeTerminals = defineStore('nativeTerminals', () => {
  const available = ref(false)
  const reason = ref('')
  const error = ref<string | null>(null)
  let probed: Promise<void> | null = null

  async function ask(): Promise<void> {
    try {
      const answer = await Available()
      available.value = answer.available
      reason.value = answer.reason
    } catch (err) {
      available.value = false
      reason.value = errorText(err, 'Native terminals are unavailable.')
    }
  }

  function probe(): Promise<void> {
    probed ??= ask()
    return probed
  }

  /** Starts a shell in the session's checkout; the address names its tab. */
  async function start(slug: string): Promise<string | null> {
    try {
      const terminal = await Start(slug)
      error.value = null
      return terminal.address
    } catch (err) {
      error.value = errorText(err, 'Could not start a native terminal.')
      return null
    }
  }

  async function kill(address: string): Promise<void> {
    try {
      await Kill(address)
    } catch (err) {
      error.value = errorText(err, 'Could not end the terminal.')
    }
  }

  return { available: readonly(available), reason: readonly(reason), error: readonly(error), probe, start, kill }
})
