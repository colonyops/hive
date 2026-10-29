import { readonly, ref } from 'vue'
import {
  ListServerSessions,
  ListSessions,
} from '../../bindings/github.com/colonyops/hive/cmd/desktop/internal/adapter/wailsui/sessionservice'
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
  /**
   * The server under servers: the session is on, '' for this machine. A
   * server's row has "<server>:" in front of its id and slug, which is how
   * every id- and slug-keyed call reaches that server.
   */
  server: string
  id: string
  name: string
  slug: string
  repo: string
  /** git.RemoteIdentity of repo: rows that share it are one repository over different transports. */
  repoIdentity: string
  state: string
  /** The owner key of the session's canvases, its repository as owner/repo. Empty for a row with none. */
  canvasOwner: string
  /** The user's tags on the session. Empty for scratch terminals and pinned chats. */
  tags: readonly string[] | null
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
export type TerminalSectionKind = 'chats' | 'scratch' | 'repo' | 'server'

/** A server under servers: as the last session list found it. */
export interface TerminalServer {
  name: string
  ssh: string
  /** connected, offline, or incompatible. */
  state: string
  error: string
}

/** One group of the sidebar tree. */
export interface TerminalSessionGroup {
  key: string
  name: string
  sessions: TerminalSessionRow[]
  kind: TerminalSectionKind
  /** Set on a `server` group. */
  server?: TerminalServer
}

/** One server's sessions, as ListServerSessions answers them. */
export interface TerminalServerSessions {
  server: TerminalServer
  sessions: TerminalSessionRow[]
}

const CHATS_GROUP_KEY = 'chats'
const SCRATCH_GROUP_KEY = 'scratch'

export function groupTerminalSessions(rows: readonly TerminalSessionRow[]): TerminalSessionGroup[] {
  const groups = new Map<string, TerminalSessionGroup>()
  for (const row of rows) {
    const key = row.repoIdentity
    let group = groups.get(key)
    if (!group) {
      group = { key, name: '', sessions: [], kind: 'repo' }
      groups.set(key, group)
    }
    group.sessions.push(row)
  }
  // Remotes in one group can be spelled differently, so the first session by
  // name picks the label, as the TUI's GroupSessionsByRepo does.
  for (const group of groups.values()) {
    group.sessions.sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id))
    group.name = groupDisplayName(group.sessions[0].repo)
  }
  return [...groups.values()].sort((a, b) => a.name.localeCompare(b.name))
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
  servers: readonly { readonly server: TerminalServer; readonly sessions: readonly TerminalSessionRow[] }[] = [],
): TerminalSessionGroup[] {
  const groups = groupTerminalSessions(rows)
  // A server is one section after the local repositories, in config order, and
  // stays in the tree while it is offline so its state is in view.
  for (const { server, sessions } of servers) {
    const active = sessions
      .filter((row) => row.state === 'active')
      .sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id))
    groups.push({
      key: `server:${server.name}`,
      name: server.name,
      sessions: active,
      kind: 'server',
      server: { ...server },
    })
  }
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

  // Apart from the local rows, so a slow or offline server never holds up the
  // local tree. Each server answers within a few seconds or reports offline.
  const servers = useResource(
    async (): Promise<TerminalServerSessions[]> =>
      ((await ListServerSessions()) ?? []).map((list) => ({
        server: { name: list.server, ssh: list.ssh, state: list.state, error: list.error },
        sessions: list.sessions ?? [],
      })),
    { initial: [] as TerminalServerSessions[], errorFallback: 'Could not list the sessions on servers.' },
  )

  // Read once: it is a constant for the run. A failure leaves the tree without
  // its scratch section rather than without its sessions.
  async function loadScratch(): Promise<void> {
    if (scratch.value) return
    try {
      const declared = await Scratch()
      if (!declared?.slug) return
      scratch.value = {
        server: '',
        id: declared.slug,
        name: declared.name,
        slug: declared.slug,
        repo: '',
        repoIdentity: '',
        state: 'active',
        canvasOwner: '',
        tags: [],
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
    const local = rows.data.value.find((row) => row.slug === slug)
    if (local) return local.repo
    for (const list of servers.data.value) {
      const row = list.sessions.find((candidate) => candidate.slug === slug)
      if (row) return row.repo
    }
    return ''
  }

  /** The server of the session at `slug`, '' for this machine. */
  function sessionServer(slug: string): string {
    const server = slug.includes(':') ? slug.slice(0, slug.indexOf(':')) : ''
    return servers.data.value.some((list) => list.server.name === server) ? server : ''
  }

  return {
    sessions: readonly(rows.data),
    scratch: readonly(scratch),
    loading: rows.loading,
    loaded: rows.loaded,
    error: rows.error,
    reload: rows.reload,
    sessionRepository,
    servers: readonly(servers.data),
    serversError: servers.error,
    reloadServers: servers.reload,
    sessionServer,
  }
})
