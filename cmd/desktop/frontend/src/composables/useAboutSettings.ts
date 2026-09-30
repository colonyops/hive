import { computed, ref } from 'vue'
import { Browser } from '@wailsio/runtime'
import { Build } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/systemservice'
import {
  CheckNow,
  SetEnabled,
  Status,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/updaterservice'
import type { BuildInfo, UpdateInfo } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'

const docsURL = 'https://hivedesktop.com/getting-started/'
const updatesDocURL = 'https://hivedesktop.com/configuration/settings/#updates'

function errText(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// useAboutSettings drives the About pane: which build is running, where it came
// from, and the auto-update controls that replace it. autoUpdate mirrors the
// persisted toggle; update holds the last check result, whose checkedAt covers
// background poll ticks as well as manual checks, so the pane can say when the
// app last looked without the user pressing anything.
export function useAboutSettings() {
  const build = ref<BuildInfo | null>(null)
  const update = ref<UpdateInfo | null>(null)
  const autoUpdate = ref(true)
  const checking = ref(false)
  const error = ref('')

  // Empty on a build with no published release — a source build reports no
  // channel, and that is also when the updater engine is absent.
  const released = computed(() => (build.value?.channel ?? '') !== '')

  async function refresh(): Promise<void> {
    error.value = ''
    try {
      const [buildInfo, status] = await Promise.all([Build(), Status()])
      build.value = buildInfo
      update.value = status
      autoUpdate.value = status.enabled
    } catch (err) {
      error.value = errText(err)
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

  async function checkForUpdates(): Promise<void> {
    checking.value = true
    error.value = ''
    try {
      update.value = await CheckNow()
    } catch (err) {
      error.value = errText(err)
    } finally {
      checking.value = false
    }
  }

  async function openExternal(url: string): Promise<void> {
    error.value = ''
    try {
      await Browser.OpenURL(url)
    } catch (err) {
      error.value = errText(err)
    }
  }

  return {
    build,
    update,
    autoUpdate,
    checking,
    error,
    released,
    refresh,
    setAutoUpdate,
    checkForUpdates,
    openDocs: () => openExternal(docsURL),
    openUpdatesDoc: () => openExternal(updatesDocURL),
  }
}
