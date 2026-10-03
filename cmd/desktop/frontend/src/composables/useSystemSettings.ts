import { errorText } from '../lib/appError'
import { ref } from 'vue'
import {
  ChooseDirectory,
  ClearConfigDir,
  ClearDataDir,
  Info,
  OpenHiveConfig,
  OpenPath,
  Quit,
  RevealPath,
  SetConfigDir,
  SetDataDir,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice'
import type { SystemInfo } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

// useSystemSettings reads the effective on-disk locations from the
// SystemService and wraps the open/reveal actions and the point-only
// data/config directory overrides. Overrides are applied on the next launch, so
// any successful change flips restartRequired.
//
// The Agents pane reads it too, for the workspace root: it is a location like
// the others, shown where the feature that owns it lives.
export function useSystemSettings() {
  const info = ref<SystemInfo | null>(null)
  const loading = ref(false)
  const error = ref('')
  const restartRequired = ref(false)

  async function refresh(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      info.value = await Info()
    } catch (err) {
      error.value = errorText(err, 'Could not load the system locations.')
    } finally {
      loading.value = false
    }
  }

  async function openPath(path: string): Promise<void> {
    error.value = ''
    try {
      await OpenPath(path)
    } catch (err) {
      error.value = errorText(err, 'Could not open the path.')
    }
  }

  async function revealPath(path: string): Promise<void> {
    error.value = ''
    try {
      await RevealPath(path)
    } catch (err) {
      error.value = errorText(err, 'Could not reveal the path.')
    }
  }

  async function createOrOpenHiveConfig(): Promise<void> {
    error.value = ''
    let openError = ''
    try {
      await OpenHiveConfig()
    } catch (err) {
      openError = errorText(err, 'Could not open the hive config.')
    }
    await refresh()
    if (openError) error.value = openError
  }

  async function changeDir(title: string, setter: (path: string) => Promise<void>): Promise<void> {
    error.value = ''
    try {
      const chosen = await ChooseDirectory(title)
      if (!chosen) return
      await setter(chosen)
      restartRequired.value = true
      await refresh()
    } catch (err) {
      error.value = errorText(err, 'Could not change the directory.')
    }
  }

  async function resetDir(clear: () => Promise<void>): Promise<void> {
    error.value = ''
    try {
      await clear()
      restartRequired.value = true
      await refresh()
    } catch (err) {
      error.value = errorText(err, 'Could not reset the directory.')
    }
  }

  return {
    info,
    loading,
    error,
    restartRequired,
    refresh,
    openPath,
    revealPath,
    createOrOpenHiveConfig,
    changeDataDir: () => changeDir('Choose data directory', SetDataDir),
    changeConfigDir: () => changeDir('Choose config directory', SetConfigDir),
    resetDataDir: () => resetDir(ClearDataDir),
    resetConfigDir: () => resetDir(ClearConfigDir),
    quit: () => {
      void Quit()
    },
  }
}
