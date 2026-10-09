import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useEventListener } from '@vueuse/core'
import { useDevTools } from '../composables/useDevTools'
import type { useFeedState } from '../composables/useFeedState'
import type { FlowsSession } from '../pipeline/composables/useFlowsSession'
import { useCodeInstallation } from '../stores/useCodeInstallation'
import { useJobs } from '../stores/useJobs'
import { ActionRunLocation } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/pipelineservice'
import {
  isApplicationSettingsSection,
  isProfileSettingsSection,
  type ApplicationSettingsSection,
  type ProfileSettingsSection,
} from '../router'
import type { SidebarSelection } from '../types/feed'

export type FeedState = ReturnType<typeof useFeedState>
export type AppNavigation = ReturnType<typeof useAppNavigation>

/**
 * The route is the app's navigation state (hash history, so native back and
 * forward traverse it). This keeps the feed state and the flows session in
 * step with it, and guards a dirty flow draft against every navigation source.
 */
export function useAppNavigation(feed: FeedState, session: FlowsSession) {
  const router = useRouter()
  const route = useRoute()
  const { activeProfileId, activeProfile, profiles, profilesLoaded, selection, unreadOnly, items } = feed
  const { enabled: devToolsEnabled, resolve: resolveDevTools } = useDevTools()

  const flowsActive = computed(() => route.name === 'flows')
  const devActive = computed(() => devToolsEnabled.value && route.name === 'dev')
  const profileSettingsActive = computed(() => route.name === 'profile-settings')
  // router.ts already rejects an unknown :section, so an unrecognized one here
  // is the absent-param case.
  const applicationSettingsSection = computed<ApplicationSettingsSection>(() =>
    isApplicationSettingsSection(route.params.section) ? route.params.section : 'general',
  )
  const profileSettingsSection = computed<ProfileSettingsSection>(() =>
    isProfileSettingsSection(route.params.section) ? route.params.section : 'general',
  )
  const canGoBack = computed(() => {
    void route.fullPath
    return router.options.history.state.back !== null
  })
  const canGoForward = computed(() => {
    void route.fullPath
    return router.options.history.state.forward !== null
  })

  // Missing profile params occur only on the first /feed load; canonicalize
  // that entry once profiles arrive so future history entries are
  // self-contained.
  watch(activeProfileId, (id) => {
    session.bindActiveFlow(id || undefined)
    const routeNeedsProfile = route.name === 'feed' || route.name === 'flows' || route.name === 'profile-settings'
    if (id && routeNeedsProfile && !route.params.profileId) {
      void router.replace({ name: route.name, params: { ...route.params, profileId: id }, query: route.query })
    }
  })

  let feedRouteSync = 0
  watch(
    [profilesLoaded, () => route.fullPath],
    async ([loaded]) => {
      if (!loaded) return
      const sync = ++feedRouteSync
      const rawProfileId = route.params.profileId
      if (typeof rawProfileId !== 'string') return
      if (!profiles.value.some((profile) => profile.id === rawProfileId)) {
        if (activeProfileId.value) void router.replace({ name: 'feed', params: { profileId: activeProfileId.value } })
        return
      }
      if (rawProfileId !== activeProfileId.value) await feed.selectProfile(rawProfileId)
      if (sync !== feedRouteSync || route.name !== 'feed') return

      const rawFeedId = route.query.feed
      const feedId =
        typeof rawFeedId === 'string' && activeProfile.value?.feeds.some((f) => f.id === rawFeedId) ? rawFeedId : null
      const wantsUnread = route.query.unread === '1'
      if (feedId) await feed.selectSidebar({ type: 'feed', feedId })
      else if (route.query.view === 'trash') await feed.selectSidebar({ type: 'trash' })
      // A bare feed route means the profile default, which is never persisted
      // as the remembered selection.
      else await feed.selectSidebar(feed.defaultSelection(rawProfileId), { persist: false })

      // selectSidebar clears the unread flag, so apply it after loading.
      if (wantsUnread && !unreadOnly.value) await feed.toggleUnread()

      // ?item is how a clicked notification lands on its item, so back and
      // forward traverse it like any other navigation.
      const wantedItem = Number(route.query.item)
      if (!Number.isSafeInteger(wantedItem) || wantedItem <= 0) return
      // An archived row lives in the lazily loaded section below the list.
      if (!items.value.some((item) => item.id === wantedItem) && !feed.archivedExpanded.value) {
        await feed.toggleArchivedSection()
        if (sync !== feedRouteSync || route.name !== 'feed') return
      }
      await feed.selectItem(wantedItem)

      const wantedRun = Number(route.query.run)
      const wantedAction = route.query.action
      if (
        typeof wantedAction === 'string' &&
        Number.isSafeInteger(wantedRun) &&
        wantedRun > 0 &&
        sync === feedRouteSync &&
        route.name === 'feed'
      )
        await feed.openActionRun(wantedItem, wantedAction, wantedRun)
    },
    { immediate: true },
  )

  watch(
    [() => route.name, () => route.query.node],
    ([name, rawNode]) => {
      if (name === 'flows') session.openFlows(typeof rawNode === 'string' ? rawNode : undefined)
      else session.exitFlows()
    },
    { immediate: true },
  )

  // A router guard, not a check in the app's buttons, so native mouse and
  // browser Back are covered too.
  const pendingNavigation = ref<{ to: string } | null>(null)
  const unsavedChangesBusy = ref(false)
  let allowGuardedNavigation = false
  onUnmounted(
    router.beforeEach((to, from) => {
      const switchesProfile = typeof to.params.profileId === 'string' && to.params.profileId !== activeProfileId.value
      const leavesDirtyFlow =
        from.name === 'flows' && (to.name !== 'flows' || to.params.profileId !== from.params.profileId)
      if (!allowGuardedNavigation && (leavesDirtyFlow || switchesProfile) && session.dirty.value) {
        pendingNavigation.value = { to: to.fullPath }
        return false
      }
    }),
  )

  function cancelPendingNavigation(): void {
    pendingNavigation.value = null
  }

  async function settlePendingNavigation(settle: () => Promise<unknown>): Promise<void> {
    if (!pendingNavigation.value) return
    unsavedChangesBusy.value = true
    await settle()
    unsavedChangesBusy.value = false
    const pending = pendingNavigation.value
    if (session.dirty.value || !pending) return
    pendingNavigation.value = null
    allowGuardedNavigation = true
    try {
      await router.push(pending.to)
    } finally {
      allowGuardedNavigation = false
    }
  }

  const deployPendingNavigation = () => settlePendingNavigation(() => session.deploy())
  const discardPendingNavigation = () => settlePendingNavigation(() => session.discardDraft())

  function openFeed(profileId = activeProfileId.value): void {
    void router.push({ name: 'feed', params: profileId ? { profileId } : {} })
  }

  function navigateSidebar(next: SidebarSelection): void {
    if (!activeProfileId.value) return
    const query = next.type === 'feed' ? { feed: next.feedId } : { view: 'trash' }
    void router.push({ name: 'feed', params: { profileId: activeProfileId.value }, query })
  }

  function navigateUnreadFilter(value: boolean): void {
    if (!activeProfileId.value) return
    const query: Record<string, string> = {}
    if (selection.value.type === 'feed') query.feed = selection.value.feedId
    else query.view = 'trash'
    if (value) query.unread = '1'
    void router.push({ name: 'feed', params: { profileId: activeProfileId.value }, query })
  }

  function openFlows(focusNodeId?: string): void {
    if (!activeProfileId.value) return
    // Immediate, for palette and canvas focus feedback; the route watcher keeps
    // it aligned during back/forward traversal.
    session.openFlows(focusNodeId)
    void router.push({
      name: 'flows',
      params: { profileId: activeProfileId.value },
      query: focusNodeId ? { node: focusNodeId } : {},
    })
  }

  async function requestSelectProfile(id: string): Promise<void> {
    // Returning from the canvas to the already-active profile does not change
    // the route profile id, so the route watcher will not reload its feeds.
    // Refresh explicitly so the sidebar cannot keep the pre-deploy flow
    // snapshot if flows:updated races the filesystem watcher.
    if (id === activeProfileId.value && !session.dirty.value) await feed.selectProfile(id)
    openFeed(id)
  }

  function openSettings(page: 'application' | 'profile'): void {
    if (page === 'application') void router.push({ name: 'application-settings' })
    else if (activeProfileId.value)
      void router.push({ name: 'profile-settings', params: { profileId: activeProfileId.value } })
  }

  function selectApplicationSettingsSection(section: ApplicationSettingsSection): void {
    void router.push({ name: 'application-settings', params: { section } })
  }

  function selectProfileSettingsSection(section: ProfileSettingsSection): void {
    if (!activeProfileId.value) return
    void router.push({ name: 'profile-settings', params: { profileId: activeProfileId.value, section } })
  }

  // The route is the attach state (ADR terminal-transport), so opening a
  // session is a navigation and nothing here touches tmux.
  const codeInstallation = useCodeInstallation()
  function openItemSession(slug: string): void {
    codeInstallation.select('local')
    void router.push({ name: 'terminal', params: { slug } })
  }

  function openItemChat(workspace: string, id: string): void {
    void router.push({ name: 'agents', params: { workspace }, query: { chat: String(id) } })
  }

  const { activeJobs } = useJobs()
  async function openJobRun(commandID: number): Promise<void> {
    const job = activeJobs.value.find((candidate) => candidate.commandId === commandID)
    if (job) await openActionRunItem(commandID, job.actionId)
  }

  async function openActionRunItem(commandID: number, actionID: string): Promise<void> {
    try {
      const location = await ActionRunLocation(commandID)
      const query: Record<string, string> = {
        item: String(location.itemId),
        action: actionID,
        run: String(commandID),
      }
      if (location.feedId) query.feed = location.feedId
      else query.view = 'trash'
      await router.push({ name: 'feed', params: { profileId: location.profileId }, query })
    } catch (error) {
      console.warn('Unable to open action run', error)
    }
  }

  // /dev is a real route in every build, so a shipped one that was not asked to
  // expose the tools leaves it once the gate answers.
  onMounted(() => {
    void resolveDevTools().then((allowed) => {
      if (!allowed && route.name === 'dev') void router.push({ name: 'feed' })
    })
  })

  // Mouse buttons 3 and 4 are Back and Forward. Route them through the router
  // so the dirty-flow guard applies, and cancel both press and auxclick
  // defaults because WebViews differ on which one navigates natively.
  const isHistoryMouseButton = (e: MouseEvent) => e.button === 3 || e.button === 4
  useEventListener(window, ['mousedown', 'auxclick'], (e: MouseEvent) => {
    if (isHistoryMouseButton(e)) e.preventDefault()
  })
  useEventListener(window, 'mouseup', (e: MouseEvent) => {
    if (!isHistoryMouseButton(e)) return
    e.preventDefault()
    if (e.button === 3) router.back()
    else router.forward()
  })

  return {
    router,
    route,
    devToolsEnabled,
    flowsActive,
    devActive,
    profileSettingsActive,
    applicationSettingsSection,
    profileSettingsSection,
    canGoBack,
    canGoForward,
    pendingNavigation,
    unsavedChangesBusy,
    cancelPendingNavigation,
    deployPendingNavigation,
    discardPendingNavigation,
    openFeed,
    navigateSidebar,
    navigateUnreadFilter,
    openFlows,
    requestSelectProfile,
    openSettings,
    selectApplicationSettingsSection,
    selectProfileSettingsSection,
    openItemSession,
    openItemChat,
    openJobRun,
    openActionRunItem,
  }
}
