import { ref } from 'vue'
import {
  Acknowledge,
  History,
  Pending,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/releasenotesservice'
import type {
  PendingReleaseNotes,
  ReleaseNote,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import { useToasts } from './useToasts'

// Module-scoped: the launch check, the toast's "What's new" action, and the
// dialog mounted once at the app root all address the same state.
const dialogOpen = ref(false)
const pendingEntries = ref<ReleaseNote[]>([])
const pendingVersion = ref('')

/**
 * useReleaseNotes drives the What's New surface. The backend decides whether
 * this launch landed on a newer version and which releases were crossed to
 * reach it; this only chooses how to say so.
 */
export function useReleaseNotes() {
  const { showToast } = useToasts()

  function openDialog(): void {
    dialogOpen.value = true
  }

  /**
   * checkOnLaunch asks the backend what this launch should show. An install
   * relaunches the app, so there is no in-process moment after an update to
   * hook — landing on a version newer than the last acknowledged one is the
   * signal.
   */
  async function checkOnLaunch(): Promise<void> {
    let pending: PendingReleaseNotes | null = null
    try {
      pending = await Pending()
    } catch {
      // Release notes are decoration: never let them break a launch.
      return
    }
    if (!pending?.show) return

    pendingEntries.value = pending.entries ?? []
    pendingVersion.value = pending.version

    if (pending.presentation === 'modal') {
      dialogOpen.value = true
      return
    }
    // The toast is the notification, so it acknowledges on sight — unlike the
    // modal, there is nothing for the user to actively dismiss, and a toast
    // that returned every launch on near-daily prereleases would be the exact
    // nuisance the toast exists to avoid.
    showToast(`Updated to ${pending.version}`, {
      body: pendingEntries.value[0]?.summary || undefined,
      severity: 'success',
      actions: [{ label: "What's new", onClick: openDialog }],
    })
    void acknowledge()
  }

  async function acknowledge(): Promise<void> {
    try {
      await Acknowledge()
    } catch {
      // A failed write only costs the user a repeat surface next launch.
    }
  }

  /** dismiss closes the modal and records the version as seen. */
  async function dismiss(): Promise<void> {
    dialogOpen.value = false
    await acknowledge()
  }

  async function loadHistory(): Promise<ReleaseNote[]> {
    try {
      return (await History()) ?? []
    } catch {
      return []
    }
  }

  return {
    dialogOpen,
    pendingEntries,
    pendingVersion,
    checkOnLaunch,
    dismiss,
    loadHistory,
  }
}
