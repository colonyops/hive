import { computed, watch, type ComputedRef, type Ref } from 'vue'
import { useStorage } from '@vueuse/core'
import { useAgentSessionsAll } from './useAgentSessionsAll'
import type { TerminalSessionRow } from './useTerminalSessions'

// Which agent chats the Code view lists above its session tree. A pin is an
// arrangement of that sidebar rather than a fact about the chat, so it is
// stored beside the tree's other layout state instead of on the chat's record —
// the same call `hive.terminal.sidebar.groups` and the running-only filter make.
//
// Pin order is insertion order, deliberately: the list is the user's, and
// re-sorting it on activity would move a row out from under the pointer.
const pinnedIds = useStorage<number[]>('hive.terminal.sidebar.pinned-chats', [])

// A chat is not a hive session, so nothing prunes its pin when the chat is
// deleted. The listing is what says it went away — but only once it has
// actually loaded: `recents` is empty both before the first read and when the
// Agents area is gated off, and pruning against either would silently discard
// every pin. reloadRecents keeps the last-good rows on failure, so a failed
// revalidation cannot look like a deletion here.
const { recents, recentsLoaded } = useAgentSessionsAll()
watch([recents, recentsLoaded], ([sessions, loaded]) => {
  if (!loaded || !pinnedIds.value.length) return
  const live = new Set(sessions.map((session) => session.id))
  const kept = pinnedIds.value.filter((id) => live.has(id))
  if (kept.length !== pinnedIds.value.length) pinnedIds.value = kept
})

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
    if (session?.slug) out.push({ id: session.slug, name: session.name, slug: session.slug, repo: '', state: 'active' })
  }
  return out
})

const slugs = computed(() => new Set(rows.value.map((row) => row.slug)))

function isPinned(id: number): boolean {
  return pinnedIds.value.includes(id)
}

function togglePin(id: number): void {
  pinnedIds.value = isPinned(id) ? pinnedIds.value.filter((pinned) => pinned !== id) : [...pinnedIds.value, id]
}

/** Unpins by slug, which is what the Code view's own rows are keyed on. */
function unpinSlug(slug: string): void {
  const session = recents.value.find((candidate) => candidate.slug === slug)
  if (session) togglePin(session.id)
}

export function useTerminalPinnedChats(): {
  pinnedIds: Ref<number[]>
  rows: ComputedRef<TerminalSessionRow[]>
  slugs: ComputedRef<Set<string>>
  isPinned: (id: number) => boolean
  togglePin: (id: number) => void
  unpinSlug: (slug: string) => void
} {
  return { pinnedIds, rows, slugs, isPinned, togglePin, unpinSlug }
}

export function resetTerminalPinnedChatsForTests(): void {
  pinnedIds.value = []
}
