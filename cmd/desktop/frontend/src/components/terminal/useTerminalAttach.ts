import { useStorage } from '@vueuse/core'
import { computed, onScopeDispose, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { setAttachedTerminalWindows } from '../../composables/useAttachedTerminalWindows'
import { useAgentWorkspaces } from '../../stores/useAgentWorkspaces'
import { useTerminalAvailability } from '../../stores/useTerminalAvailability'
import type { TerminalSessionRow } from '../../stores/useTerminalSessions'
import { selectIfWanted, type TerminalPool } from './useTerminalPool'
import type { TerminalTree, TreeWindowRow } from './useTerminalTree'

export type TerminalAttach = ReturnType<typeof useTerminalAttach>

/**
 * The URL is the attach state: /terminal/:slug is the attached session and
 * ?window its active window. Rows and restores only navigate, and the route
 * watcher here is the single path into an attach, so back/forward re-attach
 * exactly like a click. Session switches push; window changes replace, so tab
 * flips do not pile up history entries. `onSwitch` runs on every attach and
 * detach, for per-session UI state that must not carry over.
 */
export function useTerminalAttach(options: {
  pool: TerminalPool
  tree: TerminalTree
  active: () => boolean
  onSwitch: () => void
}) {
  const { pool, tree } = options
  const { activeSlug, current } = pool
  const route = useRoute()
  const router = useRouter()
  const { client, probe: probeAvailability } = useTerminalAvailability()
  const { resumeSession: resumeChat } = useAgentWorkspaces()

  const routeSlug = computed(() =>
    route.name === 'terminal' && typeof route.params.slug === 'string' ? route.params.slug : '',
  )
  const routeWindow = computed(() => (typeof route.query.window === 'string' ? route.query.window : ''))

  function terminalQuery(slug: string, window = ''): Record<string, string> {
    const query: Record<string, string> = {}
    if (window) query.window = window
    const canvas = route.name === 'terminal' ? route.query.canvas : undefined
    if (canvas !== undefined) query.canvas = slug === routeSlug.value && typeof canvas === 'string' ? canvas : '1'
    return query
  }

  // Entering bare /terminal re-attaches this instead of landing on the picker.
  const restore = useStorage('hive.terminal.restore', { slug: '', window: '' })

  const starting = ref('')
  const startError = ref('')

  // With a cached client the last-known tree renders at once and this only
  // re-checks availability. The session list is a SQLite read that knows
  // nothing about tmux, so it goes out alongside the probe rather than behind it.
  async function probe(): Promise<void> {
    const hadClient = client.value !== null
    if (hadClient) {
      if (tree.attachable.value.length) restoreLastSession()
      void tree.reloadSessions().then(restoreLastSession)
    }
    const sessions = hadClient ? null : tree.reloadSessions()
    await probeAvailability()
    if (!hadClient && client.value) {
      await sessions
      restoreLastSession()
    }
  }

  // A remembered session that no longer exists, or was recycled, is forgotten
  // rather than attached blind.
  function restoreLastSession(): void {
    if (routeSlug.value || !restore.value.slug) return
    if (!tree.attachable.value.some((row) => row.slug === restore.value.slug)) {
      restore.value = { slug: '', window: '' }
      return
    }
    void router.replace({
      name: 'terminal',
      params: { slug: restore.value.slug },
      query: restore.value.window ? { window: restore.value.window } : {},
    })
  }

  function openSession(slug: string): void {
    if (!client.value) return
    options.onSwitch()
    startError.value = ''
    pool.open(slug, client.value, routeWindow.value)
  }

  function detachSession(): void {
    options.onSwitch()
    startError.value = ''
    pool.detach()
  }

  // Immediate because the cached client can already be live at mount, in which
  // case a deep-linked slug fires no change at all.
  watch(
    [client, routeSlug],
    ([ready, slug]) => {
      if (!ready) return
      if (slug) openSession(slug)
      else detachSession()
    },
    { immediate: true },
  )

  // A same-slug push that changes only ?window (the App-level palette's window
  // rows) moves neither of the pair above.
  watch(routeWindow, (wanted) => {
    if (current.value) selectIfWanted(current.value, wanted)
  })

  // Mirror the attached window into the URL and the resume snapshot. Guarded to
  // the live route so a navigation away cannot claw the history entry back.
  watch([activeSlug, () => current.value?.activeWindowId.value ?? ''], ([slug, windowId]) => {
    if (!slug || route.name !== 'terminal' || route.params.slug !== slug) return
    restore.value = { slug, window: windowId }
    if (windowId && routeWindow.value !== windowId) {
      void router.replace({ name: 'terminal', params: { slug }, query: terminalQuery(slug, windowId) })
    }
  })

  // A list reload is how a deleted, recycled, or renamed session is noticed.
  // Hive re-slugs on rename, so following the id tells the two apart, whether
  // the change came from this window or from the hive CLI.
  const attachedId = ref('')
  watch([tree.attachable, activeSlug], ([rows, slug]) => {
    if (tree.sessionsError.value) return
    for (const pooledSlug of [...pool.pool.keys()]) {
      if (pooledSlug !== slug && !rows.some((row) => row.slug === pooledSlug)) pool.drop(pooledSlug)
    }
    if (!slug) return
    const attached = rows.find((row) => row.slug === slug)
    if (attached) {
      attachedId.value = attached.id
      return
    }
    // Attached before the list ever loaded: the attach reports its own failure.
    if (!attachedId.value) return
    const renamed = rows.find((row) => row.id === attachedId.value)
    if (!renamed) {
      closeSession()
      return
    }
    restore.value = { slug: renamed.slug, window: '' }
    void router.replace({ name: 'terminal', params: { slug: renamed.slug }, query: terminalQuery(renamed.slug) })
  })

  // The App-level palette's window rows read this projection rather than
  // component state, so they work before this async component has mounted and
  // after a trip back to the hub. It names the attached session, not the
  // visible one, which is why it follows lastAttachedSlug.
  watch(
    () => {
      const slug = pool.lastAttachedSlug.value
      const live = slug ? pool.pool.get(slug) : undefined
      const row = live ? tree.attachable.value.find((candidate) => candidate.slug === slug) : undefined
      if (!live || !row) return null
      return {
        slug: row.slug,
        name: row.name,
        windows: live.tabs.value.map((tab) => ({
          windowId: tab.windowId,
          name: tab.name || tab.windowId,
          active: tab.windowId === live.activeWindowId.value,
        })),
      }
    },
    (next) => setAttachedTerminalWindows(next),
    { immediate: true },
  )
  onScopeDispose(() => setAttachedTerminalWindows(null))

  watch(
    options.active,
    (active) => {
      if (active) void probe()
    },
    { immediate: true },
  )

  function selectSession(slug: string): void {
    if (slug === activeSlug.value) {
      // Same URL, so the route watcher stays silent. After an end the row is a
      // way back in; on a live session it is an intent to type into it.
      if (current.value?.status.value === 'ended') openSession(slug)
      else current.value?.focusActive()
      return
    }
    void router.push({ name: 'terminal', params: { slug }, query: terminalQuery(slug) })
  }

  async function goToWindow(row: TerminalSessionRow, win: TreeWindowRow): Promise<void> {
    if (win.live && row.slug === activeSlug.value) await current.value?.select(win.windowId)
    else {
      void router.push({
        name: 'terminal',
        params: { slug: row.slug },
        query: terminalQuery(row.slug, win.windowId),
      })
    }
  }

  function closeSession(): void {
    pool.drop(activeSlug.value)
    detachSession()
    restore.value = { slug: '', window: '' }
    void tree.reloadSessions()
    // Replace: Back must not walk into a session that is gone.
    if (routeSlug.value) void router.replace({ name: 'terminal' })
  }

  // Starting runs the session's agent command, so it is the user's move and
  // never a side effect of selecting a row (ADR terminal-start-is-an-offered-action).
  // A running session is not respawned, so this doubles as "open it". A pinned
  // chat starts through the Agents area's resume instead: there is no hive
  // session behind its slug to read a spawn configuration from.
  async function startSession(slug: string): Promise<void> {
    if (!client.value || starting.value) return
    starting.value = slug
    startError.value = ''
    try {
      if (tree.chatSlugs.value.has(slug)) await resumePinnedChat(slug)
      else await client.value.start(slug)
    } catch (e) {
      startError.value = e instanceof Error && e.message ? e.message : 'Could not start this session.'
      return
    } finally {
      starting.value = ''
    }
    // Nothing the sweep watches moved when a resume created the tmux session.
    if (tree.chatSlugs.value.has(slug)) tree.sweepListings()
    if (slug !== activeSlug.value) {
      void router.push({ name: 'terminal', params: { slug }, query: terminalQuery(slug) })
      return
    }
    const pooled = pool.pool.get(slug)
    if (!pooled) openSession(slug)
    else if (pooled.status.value === 'ended') void pooled.reconnect()
    else pooled.focusActive()
  }

  // The chat's id comes from the Agents area's listing rather than the slug, so
  // the core's tmux naming scheme lives in one place.
  async function resumePinnedChat(slug: string): Promise<void> {
    const session = tree.recents.value.find((candidate) => candidate.slug === slug)
    if (!session) throw new Error('This chat is no longer listed.')
    await resumeChat({ id: session.id })
    await tree.reloadRecents()
  }

  return { probe, selectSession, goToWindow, closeSession, startSession, starting, startError }
}
