import { computed, onScopeDispose, watch } from 'vue'
import { useSessionStatuses } from '../../composables/useSessionStatuses'
import { useTerminalWindowListings } from '../../composables/useTerminalWindowListings'
import { useWailsEvent } from '../../composables/useWailsEvent'
import { activityIndicator, type StatusIndicator } from '../../lib/agentActivity'
import type { WindowState } from '../../lib/terminalClient'
import { useAgentSessionsAll } from '../../stores/useAgentSessionsAll'
import { useTerminalAvailability } from '../../stores/useTerminalAvailability'
import { useTerminalPinnedChats } from '../../stores/useTerminalPinnedChats'
import {
  terminalSessionGroups,
  useTerminalSessions,
  type TerminalSessionGroup,
  type TerminalSessionRow,
} from '../../stores/useTerminalSessions'
import { useTerminalShowWindows } from '../../stores/useTerminalShowWindows'
import type { TerminalPool } from './useTerminalPool'

// One keyed list feeds a session's subtree whichever source is fresher, so the
// listed -> live swap on attach patches rows in place (window ids are stable
// across the swap) instead of unmounting one branch and mounting the other.
export interface TreeWindowRow {
  windowId: string
  name: string
  active: boolean
  live: boolean
  indicator: StatusIndicator | null
}

export type TerminalTree = ReturnType<typeof useTerminalTree>

/**
 * The session tree's model: which rows exist, whether tmux is holding each one,
 * and the windows under them. Rows come from hive's listing, the scratch
 * terminal, and the pinned agent chats. A pinned chat is a tmux session
 * addressed like a hive one (ADR agent-workspace-sessions-are-tmux-sessions),
 * but nothing in hive's listing or status projection knows about it, which is
 * the axis `isHiveSession` splits on.
 */
export function useTerminalTree(options: { pool: TerminalPool; active: () => boolean }) {
  const { pool, activeSlug } = options.pool
  const { client } = useTerminalAvailability()
  const {
    sessions: sessionRows,
    scratch: scratchRow,
    loading: sessionsLoading,
    loaded: sessionsLoaded,
    error: sessionsError,
    reload: reloadSessions,
    sessionRepository,
  } = useTerminalSessions()
  const { rows: chatRows, slugs: chatSlugs, unpinSlug } = useTerminalPinnedChats()
  const { recents, reload: reloadRecents } = useAgentSessionsAll()
  const {
    statuses: sessionStatuses,
    loaded: statusesLoaded,
    startPolling: startStatusPolling,
    stopPolling: stopStatusPolling,
  } = useSessionStatuses()
  const { showWindows: showAllWindows, ready: showAllWindowsReady } = useTerminalShowWindows()
  const { listings: sessionWindows, settled: listingsSettled, refresh: refreshListings } = useTerminalWindowListings()

  // The tree is the attach surface, so only an active session belongs in it: a
  // recycled or corrupted one has no checkout left to open a terminal in. They
  // still arrive in the listing, which is what prune counts and acts on.
  const activeSessions = computed(() => sessionRows.value.filter((row) => row.state === 'active'))

  // The scratch terminal and the pinned chats are part of this set rather than
  // beside it: the pool, the window sweep and the watcher that lets go of a
  // session the listing stopped carrying all read it. Unpinning a chat is
  // exactly that removal, which is what detaches it.
  const attachable = computed(() => [
    ...chatRows.value,
    ...(scratchRow.value ? [scratchRow.value] : []),
    ...activeSessions.value,
  ])
  const sessionGroups = computed(() => terminalSessionGroups(activeSessions.value, scratchRow.value, chatRows.value))
  const attachedRow = computed(() => attachable.value.find((row) => row.slug === activeSlug.value) ?? null)

  // The tree has no other way to learn whether tmux is holding one of these, so
  // the sweep asks about them whatever "always show windows" says.
  const alwaysSwept = computed(() => [...chatRows.value, ...(scratchRow.value ? [scratchRow.value] : [])])
  const prunableCount = computed(() => sessionRows.value.filter((row) => row.state !== 'active').length)

  function isScratch(row: TerminalSessionRow): boolean {
    return !!scratchRow.value && row.slug === scratchRow.value.slug
  }

  function isChat(row: TerminalSessionRow): boolean {
    return chatSlugs.value.has(row.slug)
  }

  function isHiveSession(row: TerminalSessionRow): boolean {
    return !isScratch(row) && !isChat(row)
  }

  // Not keyed on the attached slug: attaching cannot change another session's
  // windows, and sweeping on every switch cost the switch itself. Held while
  // off-screen (an unattached session answers a listing by spawning tmux twice)
  // and until the session list has landed (sweeping a set we know is stale
  // would report the listings settled before the real ones are in flight).
  function sweepListings(): void {
    const transport = client.value
    if (!options.active() || !transport || !sessionsLoaded.value) return
    if (showAllWindows.value) void refreshListings(transport, attachable.value)
    else if (alwaysSwept.value.length) void refreshListings(transport, alwaysSwept.value)
  }

  watch([showAllWindows, attachable, client, sessionsLoaded, options.active], sweepListings)
  watch(options.pool.endedSlugs, sweepListings)

  // Re-entry revalidates: the hub is where sessions are created, renamed and
  // deleted. The chat rows come from the Agents area's own listing, which
  // nothing else in this mode refreshes.
  watch(
    options.active,
    (active) => {
      if (!active) {
        stopStatusPolling()
        return
      }
      void reloadRecents()
      startStatusPolling()
    },
    { immediate: true },
  )
  onScopeDispose(stopStatusPolling)

  // Session creation runs as a job, so jobs:updated is what lands a launched
  // session; sessions:updated covers the CLI writing hive.db from another process.
  useWailsEvent('jobs:updated', () => void reloadSessions())
  useWailsEvent('sessions:updated', () => void reloadSessions())

  // Hive's status projection is keyed by session id and knows nothing about the
  // scratch terminal or a pinned chat, so those read their liveness off the
  // window sweep and their own live attach.
  function rowRunning(row: TerminalSessionRow): boolean {
    if (isHiveSession(row)) return !!sessionStatuses.value[row.id]?.running
    if (sessionWindows.value[row.slug]?.length) return true
    const live = pool.get(row.slug)
    return !!live && live.status.value !== 'ended'
  }

  // Greyed only once its state is known: undefined until the first status poll
  // (or window sweep) lands.
  function rowIdle(row: TerminalSessionRow): boolean {
    if (isHiveSession(row)) return sessionStatuses.value[row.id]?.running === false
    return listingsSettled.value && !rowRunning(row)
  }

  function groupAttached(group: TerminalSessionGroup): boolean {
    return group.sessions.some((row) => row.slug === activeSlug.value)
  }

  function groupRunning(group: TerminalSessionGroup): boolean {
    return group.sessions.some(rowRunning)
  }

  function windowStatus(row: TerminalSessionRow, windowId: string) {
    return sessionStatuses.value[row.id]?.windows?.find((window) => window.windowId === windowId)
  }

  function windowIndicator(row: TerminalSessionRow, windowId: string): StatusIndicator | null {
    const status = windowStatus(row, windowId)
    return status ? activityIndicator(status.status, status.tool) : null
  }

  // The scratch section is exempt from "always show windows": its tabs are the
  // section itself, and hiding them leaves a heading that says nothing.
  function listedWindows(row: TerminalSessionRow): WindowState[] {
    if (!showAllWindows.value && !isScratch(row)) return []
    return sessionWindows.value[row.slug] ?? []
  }

  // A pooled session's live tab set is fresher than its listing, but while its
  // attach is in flight the cached listing stands in so selecting a session does
  // not collapse its subtree. A chat is one conversation, so its row is a leaf.
  function buildWindowRows(row: TerminalSessionRow): TreeWindowRow[] {
    if (isChat(row)) return []
    const live = pool.get(row.slug)
    if (live?.tabs.value.length && (row.slug === activeSlug.value || showAllWindows.value || isScratch(row))) {
      return live.tabs.value.map((tab) => ({
        windowId: tab.windowId,
        name: tab.name || tab.windowId,
        active: row.slug === activeSlug.value && tab.windowId === live.activeWindowId.value,
        live: true,
        indicator: windowIndicator(row, tab.windowId),
      }))
    }
    return listedWindows(row).map((win) => ({
      windowId: win.windowId,
      name: win.name || win.windowId,
      active: false,
      live: false,
      indicator: windowIndicator(row, win.windowId),
    }))
  }

  // Keyed by session id and derived once per invalidation: the template and the
  // keyboard walk read every session's rows per render, and deriving them per
  // call re-ran the whole tree on every status poll.
  const windowRows = computed<Record<string, TreeWindowRow[]>>(() => {
    const rows: Record<string, TreeWindowRow[]> = {}
    for (const row of attachable.value) rows[row.id] = buildWindowRows(row)
    return rows
  })

  function windowRowsFor(row: TerminalSessionRow): TreeWindowRow[] {
    return windowRows.value[row.id] ?? []
  }

  return {
    activeSessions,
    attachable,
    attachedRow,
    sessionGroups,
    alwaysSwept,
    prunableCount,
    recents,
    reloadRecents,
    chatSlugs,
    unpinSlug,
    sessionsLoading,
    sessionsLoaded,
    sessionsError,
    reloadSessions,
    sessionRepository,
    statusesLoaded,
    showAllWindows,
    showAllWindowsReady,
    listingsSettled,
    refreshListings,
    sweepListings,
    isScratch,
    isChat,
    isHiveSession,
    rowRunning,
    rowIdle,
    groupAttached,
    groupRunning,
    windowStatus,
    windowRowsFor,
  }
}
