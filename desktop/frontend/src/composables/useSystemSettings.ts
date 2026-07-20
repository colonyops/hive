import { ref } from 'vue'
import {
  ChooseDirectory,
  ClearConfigDir,
  ClearDataDir,
  Info,
  OpenPath,
  Quit,
  RevealPath,
  SetConfigDir,
  SetDataDir,
} from '../../bindings/github.com/colonyops/hive/desktop/systemservice'
import type { SystemInfo } from '../../bindings/github.com/colonyops/hive/desktop/models'

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// useSystemSettings drives the System settings screen: it reads the effective
// on-disk locations from the SystemService and wraps the open/reveal actions
// and the point-only data/config directory overrides. Overrides are applied on
// the next launch, so any successful change flips restartRequired.
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
      error.value = errText(err)
    } finally {
      loading.value = false
    }
  }

  async function openPath(path: string): Promise<void> {
    error.value = ''
    try {
      await OpenPath(path)
    } catch (err) {
      error.value = errText(err)
    }
  }

  async function revealPath(path: string): Promise<void> {
    error.value = ''
    try {
      await RevealPath(path)
    } catch (err) {
      error.value = errText(err)
    }
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
      error.value = errText(err)
    }
  }

  async function resetDir(clear: () => Promise<void>): Promise<void> {
    error.value = ''
    try {
      await clear()
      restartRequired.value = true
      await refresh()
    } catch (err) {
      error.value = errText(err)
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
    changeDataDir: () => changeDir('Choose data directory', SetDataDir),
    changeConfigDir: () => changeDir('Choose config directory', SetConfigDir),
    resetDataDir: () => resetDir(ClearDataDir),
    resetConfigDir: () => resetDir(ClearConfigDir),
    quit: () => {
      void Quit()
    },
  }
}
