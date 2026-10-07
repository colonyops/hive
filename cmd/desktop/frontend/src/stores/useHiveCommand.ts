import { computed, readonly, ref } from 'vue'
import {
  SetInstall,
  Status,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/hivecliservice'
import type { HiveCLIStatus } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/app/models'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

// The `hive` command the app installs on PATH. First run asks once; Settings ▸
// Hive CLI shows the same switch and what the shell would actually run.
export const useHiveCommand = defineStore('hiveCommand', () => {
  const resource = useResource<HiveCLIStatus | null>(() => Status(), {
    initial: null,
    errorFallback: 'Could not read the hive command.',
  })
  const saving = ref(false)
  const saveError = ref<string | null>(null)

  async function setInstall(install: boolean): Promise<boolean> {
    saving.value = true
    saveError.value = null
    try {
      resource.data.value = await SetInstall(install)
      return true
    } catch (err) {
      saveError.value = errorText(err, 'Could not update the hive command.')
      return false
    } finally {
      saving.value = false
    }
  }

  void resource.reload()

  return {
    status: readonly(resource.data),
    loaded: resource.loaded,
    error: computed(() => saveError.value ?? resource.error.value),
    saving: readonly(saving),
    reload: resource.reload,
    setInstall,
  }
})
