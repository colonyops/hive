import { computed, onMounted, ref } from 'vue'
import { useWailsEvent } from '../composables/useWailsEvent'
import type { Confirmation } from '../composables/useConfirmation'
import {
  InstallUpdate,
  Status,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/updaterservice'
import type { UpdateInfo } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { FeedState } from './useAppNavigation'

/**
 * The title bar's update chip. The updater checks the release-channel manifest
 * in the background and emits update:available; Status() seeds the chip from
 * the last cached check so it does not wait for the next poll.
 */
export function useSelfUpdate(confirmation: Confirmation, showToast: FeedState['showToast']) {
  const info = ref<UpdateInfo | null>(null)
  const available = computed(() => info.value?.available ?? false)
  const latestVersion = computed(() => info.value?.latestVersion ?? '')
  const installing = ref(false)

  function openUpdate(): void {
    if (installing.value) return
    confirmation.request({
      title: 'Install update?',
      description: `Download Hive ${latestVersion.value || 'update'} and relaunch the app now?`,
      confirmLabel: 'Install and relaunch',
      testid: 'update-confirmation',
      onConfirm: install,
    })
  }

  async function install(): Promise<void> {
    installing.value = true
    try {
      await InstallUpdate()
    } catch (error) {
      installing.value = false
      console.error('Update install failed', error)
      showToast('Could not install the update', {
        severity: 'error',
        body: error instanceof Error ? error.message : String(error),
        duration: 10_000,
      })
      throw error
    }
  }

  onMounted(() => {
    void Status()
      .then((status) => {
        info.value = status
      })
      .catch((error: unknown) => {
        // eslint-disable-next-line no-console -- expected outside Wails, so not a warning
        console.debug('Updater status unavailable', error)
      })
  })
  useWailsEvent('update:available', (event: { data: UpdateInfo | UpdateInfo[] }) => {
    const payload = Array.isArray(event.data) ? event.data[0] : event.data
    if (payload) info.value = payload
  })

  return { available, latestVersion, installing, openUpdate }
}
