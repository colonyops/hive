import { computed } from 'vue'
import { useWailsEvent } from '../composables/useWailsEvent'
import type { FlowsSession } from '../pipeline/composables/useFlowsSession'
import type { ActivityItemLink } from '../lib/activityPresentation'
import {
  Feed,
  FindItems,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import type {
  MenuBarNavigation,
  NotificationActivation,
  NotificationToast,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/models'
import type { AppNavigation, FeedState } from './useAppNavigation'

type Payload<T> = { data: T | T[] }
const first = <T>(event: Payload<T>): T | undefined => (Array.isArray(event.data) ? event.data[0] : event.data)

const toastSeverities = ['info', 'success', 'warning', 'error'] as const
type ToastSeverity = (typeof toastSeverities)[number]
// An unrecognized severity reads as info rather than being dropped.
const toastSeverity = (severity: string): ToastSeverity =>
  toastSeverities.includes(severity as ToastSeverity) ? (severity as ToastSeverity) : 'info'

/**
 * Everything outside the feed that points at a place in it: a clicked
 * notification, the menu bar, an Activity row, and the title bar's error chip.
 * The window is already raised by the time an event lands here, so routing is
 * all that is left.
 */
export function useReveal(nav: AppNavigation, session: FlowsSession, showToast: FeedState['showToast']) {
  // Sourced from the always-on session, so the chip deep-links with the
  // canvas closed.
  const errorNodeIds = computed(() =>
    (session.activeFlow.value?.nodes ?? [])
      .filter((node) => session.latestRunByNode.value.get(node.id)?.ok === false)
      .map((node) => node.id),
  )
  const errorCount = computed(() => errorNodeIds.value.length)

  function openErrorNode(): void {
    const nodeId = errorNodeIds.value[0]
    if (nodeId) nav.openFlows(nodeId)
  }

  async function revealInboxItem(profileId: string, itemId: number): Promise<void> {
    let feedId = ''
    try {
      feedId = (await Feed(profileId, itemId)) ?? ''
    } catch (error) {
      console.warn('Unable to locate the inbox item', error)
    }
    const query: Record<string, string> = { item: String(itemId) }
    if (feedId) query.feed = feedId
    else query.view = 'trash'
    await nav.router.push({ name: 'feed', params: { profileId }, query })
  }

  // An item id of 0 means the notification named none, so it lands on the
  // profile.
  async function revealNotification(activation: NotificationActivation): Promise<void> {
    const profileId = activation.profileId
    if (!profileId) return
    const itemId = Number(activation.itemId ?? 0)
    if (!Number.isSafeInteger(itemId) || itemId <= 0) {
      nav.openFeed(profileId)
      return
    }
    await revealInboxItem(profileId, itemId)
  }

  async function openFromMenuBar(menu: MenuBarNavigation): Promise<void> {
    if (menu.settings) {
      nav.selectApplicationSettingsSection('menubar')
      return
    }
    if (!menu.profileId) return
    if (menu.itemId > 0) {
      await revealInboxItem(menu.profileId, menu.itemId)
      return
    }
    await nav.router.push({
      name: 'feed',
      params: { profileId: menu.profileId },
      query: menu.feedId ? { feed: menu.feedId } : {},
    })
  }

  async function openActivityItem(link: ActivityItemLink): Promise<void> {
    try {
      const candidates = (await FindItems(link.profileId, link.externalId)) ?? []
      const exact = candidates.find(
        (item) => item.sourceKind === link.sourceKind && item.sourceScope === link.sourceScope,
      )
      const item = exact ?? (candidates.length === 1 ? candidates[0] : undefined)
      if (!item) {
        showToast('Could not find the linked item', { severity: 'error' })
        return
      }
      await revealInboxItem(link.profileId, item.id)
    } catch (error) {
      console.warn('Unable to open the linked activity item', error)
      showToast('Could not open the linked item', { severity: 'error' })
    }
  }

  useWailsEvent('notification:activated', (event: Payload<NotificationActivation>) => {
    const payload = first(event)
    if (payload) void revealNotification(payload)
  })
  useWailsEvent('menubar:open', (event: Payload<MenuBarNavigation>) => {
    const payload = first(event)
    if (payload) void openFromMenuBar(payload)
  })
  // A flow notification the user chose to receive in-app rather than as an OS
  // banner. Go has already applied the kill switch and picked this channel.
  useWailsEvent('notification:toast', (event: Payload<NotificationToast>) => {
    const payload = first(event)
    if (payload?.title) showToast(payload.title, { body: payload.body, severity: toastSeverity(payload.severity) })
  })

  return { errorCount, openErrorNode, openActivityItem }
}
