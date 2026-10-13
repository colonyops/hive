import { computed } from 'vue'
import { useCodeInstallation } from '../stores/useCodeInstallation'
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
import { Take as TakeLink } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/linkservice'
import type { Location } from '../../bindings/github.com/colonyops/hive/internal/domain/deeplink/models'
import { canvasDocumentPath, ownerToSpaceKey, type DriveScope } from '../lib/drive'
import type { AppNavigation, FeedState } from './useAppNavigation'
import { createLogger } from '../lib/log'

const log = createLogger('reveal')

type Payload<T> = { data: T | T[] }
const first = <T>(event: Payload<T>): T | undefined => (Array.isArray(event.data) ? event.data[0] : event.data)

const toastSeverities = ['info', 'success', 'warning', 'error'] as const
type ToastSeverity = (typeof toastSeverities)[number]
// An unrecognized severity reads as info rather than being dropped.
const toastSeverity = (severity: string): ToastSeverity =>
  toastSeverities.includes(severity as ToastSeverity) ? (severity as ToastSeverity) : 'info'

/**
 * Everything outside a view that points at a place in the app: a clicked
 * notification, the menu bar, an Activity row, the title bar's error chip, and
 * a hive:// link. The window is already raised by the time an event lands
 * here, so routing is all that is left.
 */
export function useReveal(
  nav: AppNavigation,
  session: FlowsSession,
  showToast: FeedState['showToast'],
  openDrive: (scope: DriveScope) => void,
) {
  const codeInstallation = useCodeInstallation()
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
      log.warn('unable to locate the inbox item', error)
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
      log.warn('unable to open the linked activity item', error)
      showToast('Could not open the linked item', { severity: 'error' })
    }
  }

  // A canvas link from before Hive Drive names the canvas, not its document.
  // A bare '1' opens the pane and lets it pick.
  const docQuery = (canvas: string) => (canvas === '1' ? '1' : canvasDocumentPath(canvas))

  // Go has already parsed and validated the link, so every field its area
  // needs is present.
  async function revealLocation(loc: Location): Promise<void> {
    switch (loc.area) {
      case 'code': {
        codeInstallation.select('local')
        const query: Record<string, string> = {}
        if (loc.window) query.window = loc.window
        if (loc.canvas) query.doc = docQuery(loc.canvas)
        await nav.router.push({ name: 'terminal', params: { slug: loc.slug }, query })
        return
      }
      case 'chats': {
        const query: Record<string, string> = {}
        if (loc.chat) query.chat = loc.chat
        if (loc.canvas) query.doc = docQuery(loc.canvas)
        await nav.router.push({ name: 'agents', params: { workspace: loc.workspace }, query })
        return
      }
      case 'inbox':
        if (loc.item) await revealInboxItem(loc.profile ?? '', loc.item)
        else
          await nav.router.push({
            name: 'feed',
            params: { profileId: loc.profile },
            query: loc.feed ? { feed: loc.feed } : {},
          })
        return
      case 'canvas':
        openDrive({
          space: ownerToSpaceKey(loc.owner ?? ''),
          path: loc.canvas ? canvasDocumentPath(loc.canvas) : null,
          session: null,
        })
        return
      case 'drive':
        openDrive({ space: loc.space ?? '', path: loc.path || null, session: null })
        return
      default:
        showToast(`Hive cannot open ${loc.area} links yet`, { severity: 'error' })
    }
  }

  async function openPendingLink(): Promise<void> {
    try {
      const loc = await TakeLink()
      if (loc) await revealLocation(loc)
    } catch (error) {
      log.warn('unable to open the link', error)
      showToast('Could not open the link', { severity: 'error' })
    }
  }

  // The listener only catches links that arrive while the app runs. A link that
  // launched the app arrived before it, so the pending one is taken once now.
  useWailsEvent('link:open', () => void openPendingLink())
  void openPendingLink()

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
