import { computed, readonly, shallowRef, watch } from 'vue'
import { useStorage } from '@vueuse/core'
import { useWailsEvent } from '../composables/useWailsEvent'
import { defineStore } from './defineStore'
import { useAgentSessionsAll } from './useAgentSessionsAll'
import type { TerminalSessionRow } from './useTerminalSessions'

// Which agent chats the Code view lists above its session tree. A pin is an
// arrangement of that sidebar rather than a fact about the chat, so it is
// stored beside the tree's other layout state instead of on the chat's record —
// the same call `hive.terminal.sidebar.groups` and the active Code view make.
//
// Pin order is insertion order, deliberately: the list is the user's, and
// re-sorting it on activity would move a row out from under the pointer.
export const useTerminalPinnedChats = defineStore('terminalPinnedChats', () => {
  const pinnedIds = useStorage<string[]>('hive.terminal.sidebar.pinned-chats', [])

  // A chat is not a hive session, so nothing prunes its pin when the chat is
  // deleted. The listing is what says it went away — but only once it has
  // actually loaded: `recents` is empty both before the first read and when the
  // Agents area is gated off, and pruning against either would silently discard
  // every pin. The listing keeps its last-good rows on failure, so a failed
  // revalidation cannot look like a deletion here.
  const { recents, recentsLoaded, reload: reloadRecents } = useAgentSessionsAll()
  watch(
    [recents, recentsLoaded],
    ([sessions, loaded]) => {
      if (!loaded || !pinnedIds.value.length) return
      const canonicalByID = new Map<string, string>()
      for (const session of sessions) {
        canonicalByID.set(session.id, session.id)
        if (session.legacyId) canonicalByID.set(session.legacyId, session.id)
      }
      const seen = new Set<string>()
      const kept: string[] = []
      for (const storedID of pinnedIds.value) {
        const canonicalID = canonicalByID.get(String(storedID))
        if (canonicalID && !seen.has(canonicalID)) {
          kept.push(canonicalID)
          seen.add(canonicalID)
        }
      }
      if (kept.length !== pinnedIds.value.length || kept.some((id, index) => id !== pinnedIds.value[index])) {
        pinnedIds.value = kept
      }
    },
    { immediate: true },
  )

  /**
   * The pinned chats as sidebar rows, in pin order. A chat's slug is the tmux
   * session name the core declares whether or not it is running, so a row is the
   * same row — poolable, routable, keyboard-reachable — either way; its liveness
   * is the sidebar's own window sweep to answer, exactly as for the scratch
   * terminal. A pin whose chat the listing has not produced yet draws nothing
   * rather than a row with no slug to attach.
   */
  const rows = computed<TerminalSessionRow[]>(() => {
    const byID = new Map(recents.value.map((session) => [session.id, session]))
    const out: TerminalSessionRow[] = []
    for (const id of pinnedIds.value) {
      const session = byID.get(id)
      if (session?.slug)
        out.push({
          id: session.slug,
          name: session.name,
          slug: session.slug,
          repo: '',
          repoIdentity: '',
          state: 'active',
          canvasOwner: '',
          tags: [],
          createdAt: '',
        })
    }
    return out
  })

  const slugs = computed(() => new Set(rows.value.map((row) => row.slug)))

  function isPinned(id: string): boolean {
    return pinnedIds.value.includes(id)
  }

  function togglePin(id: string): void {
    pinnedIds.value = isPinned(id) ? pinnedIds.value.filter((pinned) => pinned !== id) : [...pinnedIds.value, id]
  }

  function pin(id: string): void {
    if (!isPinned(id)) pinnedIds.value = [...pinnedIds.value, id]
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
      if (match) pin(match.id)
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
  function unpinSlug(slug: string): void {
    const session = recents.value.find((candidate) => candidate.slug === slug)
    if (session) togglePin(session.id)
  }

  return { pinnedIds: readonly(pinnedIds), rows, slugs, isPinned, togglePin, pin, pinWhenListed, unpinSlug }
})
