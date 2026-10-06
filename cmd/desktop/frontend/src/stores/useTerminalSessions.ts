import { readonly, ref } from 'vue'
import { ListSessions } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
import { Scratch } from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/terminalservice'
import { repoDisplayName } from '../lib/repositories'
import { defineStore } from './defineStore'
import { useResource } from './useResource'

/**
 * One row of the terminal sidebar. Slug is the tmux target an attach uses; a
 * session with no tmux session yet is offered a start rather than failing, so
 * no row is dead. Only an `active` session can be started or attached to —
 * recycled and corrupted ones are listed so they can be read and deleted.
 */
export interface TerminalSessionRow {
  id: string
  name: string
  slug: string
  repo: string
  state: string
  /** The owner key of the session's canvases, its repository as owner/repo. Empty for a row with none. */
  canvasOwner: string
  /** Start of the current Hive session lifecycle. Empty for scratch terminals and pinned chats. */
  createdAt: string
}

/**
 * What a section of the sidebar tree is. `repo` is a hive remote and every
 * other kind belongs to no repository: `chats` lists the pinned agent chats and
 * `scratch` is the scratch terminal's own section. The distinction is not
 * cosmetic — a repo's liveness comes from hive's status projection, the others'
 * from tmux directly — and it is a closed union rather than a `pinned` flag
 * because the two non-repo kinds do not render alike.
 */
export type TerminalSectionKind = 'chats' | 'scratch' | 'repo'

/** One group of the sidebar tree. */
export interface TerminalSessionGroup {
  key: string
  name: string
  sessions: TerminalSessionRow[]
  kind: TerminalSectionKind
}

// The non-repo sections' keys. A repo group is keyed by its remote, which can
// never collide with either of these.
const CHATS_GROUP_KEY = 'chats'
const SCRATCH_GROUP_KEY = 'scratch'

// Mirrors the TUI's GroupSessionsByRepo: one group per remote, a "(no remote)"
// group for sessions without one, groups and sessions alphabetical.
export function groupTerminalSessions(rows: readonly TerminalSessionRow[]): TerminalSessionGroup[] {
  const groups = new Map<string, TerminalSessionGroup>()
  for (const row of rows) {
    let group = groups.get(row.repo)
    if (!group) {
      group = { key: row.repo, name: groupDisplayName(row.repo), sessions: [], kind: 'repo' }
      groups.set(row.repo, group)
    }
    group.sessions.push(row)
  }
  const sorted = [...groups.values()].sort((a, b) => a.name.localeCompare(b.name))
  for (const group of sorted) group.sessions.sort((a, b) => a.name.localeCompare(b.name))
  return sorted
}

function groupDisplayName(remote: string): string {
  return repoDisplayName(remote) || '(no remote)'
}

/**
 * The tree's groups: pinned chats, then the scratch terminal, then one per
 * repository. The two leading sections are prepended rather than sorted in
 * because being pinned is the point — they must not move as repositories come
 * and go — and chats lead because a pinned chat is something the user is
 * watching, which is the whole reason it was pinned.
 *
 * The scratch section is a group of one so the tree stays group → session →
 * window everywhere, and it draws no header: its row is the heading, and what is
 * listed under it are the tabs. The chats section is the ordinary shape — a
 * header over its rows — and is absent entirely when nothing is pinned, so the
 * sidebar is unchanged for anyone not using it.
 */
export function terminalSessionGroups(
  rows: readonly TerminalSessionRow[],
  scratch: TerminalSessionRow | null,
  chats: TerminalSessionRow[],
): TerminalSessionGroup[] {
  const groups = groupTerminalSessions(rows)
  if (scratch) groups.unshift({ key: SCRATCH_GROUP_KEY, name: scratch.name, sessions: [scratch], kind: 'scratch' })
  if (chats.length) groups.unshift({ key: CHATS_GROUP_KEY, name: 'Chats', sessions: chats, kind: 'chats' })
  return groups
}

// The rows are hive's session set, not one view's, and the tree renders the
// last-known ones while reload() revalidates rather than emptying and popping
// back in. The store does not load itself: terminal mode revalidates on every
// visit, and the first visit is the first read.
export const useTerminalSessions = defineStore('terminalSessions', () => {
  // The scratch terminal as a row of the same shape, so everything keyed on a
  // session — the pool, the window listings, the keyboard walk — reaches it
  // without learning a second kind of row. It carries no repo and its slug is
  // its id, which no hive session id can collide with. Declared by the core
  // rather than assumed here, because the slug is what an attach addresses.
  const scratch = ref<TerminalSessionRow | null>(null)

  const rows = useResource(
    async () => {
      const [list] = await Promise.all([ListSessions(), loadScratch()])
      return list ?? []
    },
    { initial: [] as TerminalSessionRow[], errorFallback: 'Could not list sessions.' },
  )

  // Read once: it is a constant for the run. A failure leaves the tree without
  // its scratch section rather than without its sessions.
  async function loadScratch(): Promise<void> {
    if (scratch.value) return
    try {
      const declared = await Scratch()
      if (!declared?.slug) return
      scratch.value = {
        id: declared.slug,
        name: declared.name,
        slug: declared.slug,
        repo: '',
        state: 'active',
        canvasOwner: '',
        createdAt: '',
      }
    } catch {
      // Terminal mode reports its own unavailability; a missing scratch row is
      // not worth failing the session list over.
    }
  }

  /**
   * The remote of the session at `slug`, or '' when there is no such session or
   * it has none. This is what makes "new session" default to the repository you
   * are already working in rather than to the first workspace on disk.
   */
  function sessionRepository(slug: string): string {
    if (!slug) return ''
    return rows.data.value.find((row) => row.slug === slug)?.repo ?? ''
  }

  return {
    sessions: readonly(rows.data),
    scratch: readonly(scratch),
    loading: rows.loading,
    loaded: rows.loaded,
    error: rows.error,
    reload: rows.reload,
    sessionRepository,
  }
})
