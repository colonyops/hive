import { computed, shallowRef, watch } from 'vue'
import { useToasts } from '../composables/useToasts'
import { useWailsEvent } from '../composables/useWailsEvent'
import { errorText } from '../lib/appError'
import { defineStore } from './defineStore'
import { useAgentSessionsAll } from './useAgentSessionsAll'
import { useAgentWorkspaces } from './useAgentWorkspaces'
import type { TerminalSessionRow } from './useTerminalSessions'

const legacyPinsKey = 'hive.terminal.sidebar.pinned-chats'

function readLegacyPins(): string[] | null {
  try {
    const raw = localStorage.getItem(legacyPinsKey)
    if (raw === null) return null
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.map(String) : []
  } catch {
    return null
  }
}

function forgetLegacyPins(): void {
  try {
    localStorage.removeItem(legacyPinsKey)
  } catch {
    // Storage that cannot be written cannot import again either.
  }
}

// Which agent chats the Code view lists above its session tree. The chat
// record holds the pin, so an agent, an applet or `hive dt chat-pin` can pin a
// chat too. Pin order is pin time, which keeps the user's insertion order:
// re-sorting on activity would move a row out from under the pointer.
export const useTerminalPinnedChats = defineStore('terminalPinnedChats', () => {
  const { recents, recentsLoaded, reload: reloadRecents } = useAgentSessionsAll()

  const pinnedSessions = computed(() =>
    recents.value
      .filter((session) => session.pinnedAt > 0)
      .sort((a, b) => a.pinnedAt - b.pinnedAt || a.id.localeCompare(b.id)),
  )
  const pinnedIds = computed(() => pinnedSessions.value.map((session) => session.id))

  // Pins from before the record held them. They are imported once, after the
  // listing loads, because the import needs the client the listing probes for.
  let importStarted = false
  watch(
    recentsLoaded,
    async (loaded) => {
      if (!loaded || importStarted) return
      const legacy = readLegacyPins()
      if (legacy === null) return
      importStarted = true
      try {
        const { client } = useAgentWorkspaces()
        if (!client.value) return
        if (legacy.length) await client.value.importPins(legacy)
        forgetLegacyPins()
        if (legacy.length) await reloadRecents()
      } catch (e) {
        importStarted = false
        useToasts().showToast(errorText(e, 'Could not import pinned chats.'), { severity: 'error' })
      }
    },
    { immediate: true },
  )

  /**
   * The pinned chats as sidebar rows, in pin order. A chat's slug is the tmux
   * session name the core declares whether or not it is running, so a row is the
   * same row — poolable, routable, keyboard-reachable — either way; its liveness
   * is the sidebar's own window sweep to answer, exactly as for the scratch
   * terminal.
   */
  const rows = computed<TerminalSessionRow[]>(() =>
    pinnedSessions.value
      .filter((session) => session.slug)
      .map((session) => ({
        server: '',
        id: session.slug,
        name: session.name,
        slug: session.slug,
        repo: '',
        repoIdentity: '',
        state: 'active',
        driveSpace: '',
        tags: [],
        createdAt: '',
      })),
  )

  const slugs = computed(() => new Set(rows.value.map((row) => row.slug)))

  function isPinned(id: string): boolean {
    return pinnedIds.value.includes(id)
  }

  async function setPinned(id: string, pinned: boolean): Promise<void> {
    try {
      const { client, ready } = useAgentWorkspaces()
      await ready()
      if (!client.value) throw new Error('The Agents area is unavailable.')
      await client.value.pinSession(id, pinned)
      await reloadRecents()
    } catch (e) {
      useToasts().showToast(errorText(e, pinned ? 'Could not pin the chat.' : 'Could not unpin the chat.'), {
        severity: 'error',
      })
    }
  }

  function togglePin(id: string): Promise<void> {
    return setPinned(id, !isPinned(id))
  }

  function pin(id: string): Promise<void> {
    return isPinned(id) ? Promise.resolve() : setPinned(id, true)
  }

  // A chat created as a background job has no id until the job lands it in the
  // listing, so its pin waits for the first chat with its workspace and name
  // that was not already listed when the pin was asked for.
  interface AwaitedPin {
    workspace: string
    name: string
    existing: Set<string>
  }
  const awaited = shallowRef<AwaitedPin[]>([])

  function pinWhenListed(workspace: string, name: string): void {
    const existing = new Set(
      recents.value.filter((session) => session.workspace === workspace && session.name === name).map((s) => s.id),
    )
    awaited.value = [...awaited.value, { workspace, name, existing }]
  }

  watch([recents, awaited], ([sessions, waiting]) => {
    if (!waiting.length) return
    const still = waiting.filter((want) => {
      const match = sessions.find(
        (session) =>
          session.workspace === want.workspace && session.name === want.name && !want.existing.has(session.id),
      )
      if (match) void pin(match.id)
      return !match
    })
    if (still.length !== waiting.length) awaited.value = still
  })

  // The listing does not reload itself; jobs:updated is what says a create job
  // finished.
  useWailsEvent('jobs:updated', () => {
    if (awaited.value.length) void reloadRecents()
  })

  /** Unpins by slug, which is what the Code view's own rows are keyed on. */
  function unpinSlug(slug: string): Promise<void> {
    const session = recents.value.find((candidate) => candidate.slug === slug)
    return session ? setPinned(session.id, false) : Promise.resolve()
  }

  return {
    pinnedIds,
    rows,
    slugs,
    isPinned,
    togglePin,
    pin,
    pinWhenListed,
    unpinSlug,
  }
})
