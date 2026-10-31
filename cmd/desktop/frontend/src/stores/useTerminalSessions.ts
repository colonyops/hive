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
  /**
   * A server's row from its last answer, kept while the server does not
   * answer: it stays in place so the tree does not reshuffle, but nothing can
   * attach to it.
   */
  offline?: boolean
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
  /** The servers with a session in this repository, in config order. */
  servers?: string[]
}

export interface TerminalServerSessions {
  server: TerminalServer
  sessions: TerminalSessionRow[]
}

const CHATS_GROUP_KEY = 'chats'
const SCRATCH_GROUP_KEY = 'scratch'

/**
 * The tree's repositories, each with this machine's sessions first and then
 * each server's in `servers` order. A server's session joins the group of
 * its repository, so one repository is one group wherever its sessions run.
 */
export function groupTerminalSessions(
  rows: readonly TerminalSessionRow[],
  servers: readonly string[] = [],
): TerminalSessionGroup[] {
  const groups = new Map<string, TerminalSessionGroup>()
  for (const row of rows) {
    const key = row.repoIdentity
    let group = groups.get(key)
    if (!group) {
      group = { key, name: '', sessions: [], kind: 'repo', servers: [] }
      groups.set(key, group)
    }
    group.sessions.push(row)
  }
  const hostOrder = (row: TerminalSessionRow) => (row.server ? servers.indexOf(row.server) + 1 : 0)
  // Remotes in one group can be spelled differently, so the first session by
  // name picks the label, as the TUI's GroupSessionsByRepo does.
  for (const group of groups.values()) {
    const byName = [...group.sessions].sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id))
    group.name = groupDisplayName(byName[0].repo)
    group.sessions = byName.sort((a, b) => hostOrder(a) - hostOrder(b))
    group.servers = servers.filter((name) => group.sessions.some((row) => row.server === name))
  }
  return [...groups.values()].sort((a, b) => a.name.localeCompare(b.name))
}

const SERVER_HUES = [
  { mark: 'bg-node-blue-tint text-node-blue', fill: 'bg-node-blue' },
  { mark: 'bg-node-purple-tint text-node-purple', fill: 'bg-node-purple' },
  { mark: 'bg-node-teal-tint text-node-teal', fill: 'bg-node-teal' },
  { mark: 'bg-node-orange-tint text-node-orange', fill: 'bg-node-orange' },
  { mark: 'bg-node-green-tint text-node-green', fill: 'bg-node-green' },
] as const

export type ServerHue = (typeof SERVER_HUES)[number]

/**
 * A server's color, by its place in `servers`, so it stays put while the
 * server is offline. Colors repeat after five servers.
 */
export function serverHue(servers: readonly string[], name: string): ServerHue {
  const index = Math.max(servers.indexOf(name), 0)
  return SERVER_HUES[index % SERVER_HUES.length]
}

function groupDisplayName(remote: string): string {
  return repoDisplayName(remote) || '(no remote)'
}

/**
 * The tree's groups: pinned chats, then the scratch terminal, then one per
 * repository, holding that repository's sessions here and on every server.
 * The two leading sections are prepended rather than sorted in
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
  const serverRows = servers.flatMap(({ sessions }) => sessions.filter((row) => row.state === 'active'))
  const groups = groupTerminalSessions(
    [...rows, ...serverRows],
    servers.map(({ server }) => server.name),
  )
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

  const rows = useResource<TerminalSessionRow[]>(
    async () => {
      const [list] = await Promise.all([ListSessions(), loadScratch()])
      return list ?? []
    },
    { initial: [] as TerminalSessionRow[], errorFallback: 'Could not list sessions.' },
  )

  // Apart from the local rows, so a slow or offline server never holds up the
  // local tree. Each server answers within a few seconds or reports offline.
  // A server that stops answering keeps the sessions it last listed, marked
  // offline.
  const servers = useResource(
    async (): Promise<TerminalServerSessions[]> => {
      const previous = new Map(servers.data.value.map((list) => [list.server.name, list.sessions]))
      return ((await ListServerSessions()) ?? []).map((list): TerminalServerSessions => {
        const server = { name: list.server, ssh: list.ssh, state: list.state, error: list.error }
        if (list.state === 'connected') return { server, sessions: list.sessions ?? [] }
        const lastKnown = previous.get(list.server) ?? []
        return { server, sessions: lastKnown.map((row) => ({ ...row, offline: true })) }
      })
    },
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
    serversLoaded: servers.loaded,
    reloadServers: servers.reload,
    sessionServer,
  }
})
