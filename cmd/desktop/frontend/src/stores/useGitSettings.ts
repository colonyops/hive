import { computed, readonly, ref } from 'vue'
import {
  CloneProtocol,
  SetCloneProtocol,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/settingsservice'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

export const useGitSettings = defineStore('gitSettings', () => {
  const configured = useResource(CloneProtocol, {
    initial: 'https',
    errorFallback: 'Could not load the clone protocol.',
  })
  const saveError = ref<string | null>(null)
  void configured.reload()

  async function setProtocol(value: string): Promise<void> {
    saveError.value = null
    try {
      await SetCloneProtocol(value)
      await configured.reload()
    } catch (error) {
      saveError.value = errorText(error, 'Could not save the clone protocol.')
    }
  }

  return {
    protocol: readonly(configured.data),
    error: computed(() => saveError.value ?? configured.error.value),
    reload: configured.reload,
    setProtocol,
  }
})
