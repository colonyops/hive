import { ref } from 'vue'
import { Browser } from '@wailsio/runtime'
import {
  Build,
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
import {
  CheckNow,
  SetEnabled,
  Status,
} from '../../bindings/github.com/colonyops/hive/desktop/updaterservice'
import type { BuildInfo, SystemInfo, UpdateInfo } from '../../bindings/github.com/colonyops/hive/desktop/models'

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// useSystemSettings drives the System settings screen: it reads the effective
// on-disk locations from the SystemService and wraps the open/reveal actions
// and the point-only data/config directory overrides. Overrides are applied on
// the next launch, so any successful change flips restartRequired.
export function useSystemSettings() {
  const info = ref<SystemInfo | null>(null)
  const build = ref<BuildInfo | null>(null)
  const loading = ref(false)
  const error = ref('')
  const restartRequired = ref(false)

  // Auto-update state. autoUpdate mirrors the persisted toggle; update holds
  // the last check result so the view can render "up to date" / "vX available"
  // inline. checkingUpdate guards the manual Check button; checkedOnce lets the
  // view distinguish "never checked" from an up-to-date result.
  const autoUpdate = ref(true)
  const update = ref<UpdateInfo | null>(null)
  const checkingUpdate = ref(false)
  const checkedOnce = ref(false)

  async function refresh(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const [locations, buildInfo, status] = await Promise.all([Info(), Build(), Status()])
      info.value = locations
      build.value = buildInfo
      update.value = status
      autoUpdate.value = status.enabled
    } catch (err) {
      error.value = errText(err)
    } finally {
      loading.value = false
    }
  }

  // setAutoUpdate persists the toggle through the service (which also starts or
  // stops the background ticker). On failure the previous value is restored so
  // the switch never drifts from the backend.
  async function setAutoUpdate(value: boolean): Promise<void> {
    const previous = autoUpdate.value
    autoUpdate.value = value
    error.value = ''
    try {
      await SetEnabled(value)
    } catch (err) {
      autoUpdate.value = previous
      error.value = errText(err)
    }
  }

  // checkForUpdates runs a manual check and stores the result for inline
  // display next to the Version row.
  async function checkForUpdates(): Promise<void> {
    checkingUpdate.value = true
    error.value = ''
    try {
      update.value = await CheckNow()
      checkedOnce.value = true
    } catch (err) {
      error.value = errText(err)
    } finally {
      checkingUpdate.value = false
    }
  }

  // openExternal opens a build-info URL in the system browser. Guarded by a
  // non-empty url (dev builds have no releaseUrl), so the view only wires links
  // when there is somewhere to go.
  async function openExternal(url: string | undefined): Promise<void> {
    if (!url) return
    error.value = ''
    try {
      await Browser.OpenURL(url)
    } catch (err) {
      error.value = errText(err)
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
    build,
    loading,
    error,
    restartRequired,
    autoUpdate,
    update,
    checkingUpdate,
    checkedOnce,
    setAutoUpdate,
    checkForUpdates,
    refresh,
    openReleaseNotes: () => openExternal(build.value?.releaseUrl),
    openRepo: () => openExternal(build.value?.repoUrl),
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
