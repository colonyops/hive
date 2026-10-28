import { computed, ref, watchEffect } from 'vue'
import { useTreeExpansion } from '../../composables/useTreeExpansion'
import { filterReadsStatus, matchesSessionFilter, type SessionFilter } from '../../lib/sessionViews'
import type { TerminalSessionGroup } from '../../stores/useTerminalSessions'
import type { TerminalTree } from './useTerminalTree'

function countSessions(groups: TerminalSessionGroup[]): number {
  return groups.reduce((total, group) => total + group.sessions.length, 0)
}

export type SessionTreeView = ReturnType<typeof useSessionTreeView>

/**
 * What the session tree draws: the saved view's narrowing, the name query, the
 * folds, and when the first fill may paint. Narrowing only changes what is
 * drawn; `attachable` keeps every session, because a session filtered off the
 * screen must not read as one that went away.
 */
export function useSessionTreeView(
  tree: Pick<
    TerminalTree,
    | 'sessionGroups'
    | 'activeSessions'
    | 'attachable'
    | 'alwaysSwept'
    | 'rowRunning'
    | 'groupAttached'
    | 'groupRunning'
    | 'statusesLoaded'
    | 'sessionsLoaded'
    | 'sessionsError'
    | 'showAllWindows'
    | 'showAllWindowsReady'
    | 'listingsSettled'
  >,
  options: {
    /** The active view's filter. */
    filter?: () => SessionFilter
    /** False while the filter may still change under a stored view; the first paint waits for it. */
    filterReady?: () => boolean
  } = {},
) {
  // Transient on purpose: a filter restored at launch hides sessions the user
  // has no reason to suspect are there. A saved view may persist because the
  // view bar always shows its chips and counts what they hide.
  const query = ref('')

  // Taken before the query, so a repo-name match carries what is left. Only
  // repo groups narrow: scratch is the tree's one always-there way to open a
  // shell, and a chat was pinned to be kept in view.
  const viewGroups = computed<TerminalSessionGroup[]>(() => {
    const filter = options.filter?.()
    if (!filter?.conditions.length) return tree.sessionGroups.value
    const ctx = { rowRunning: tree.rowRunning, statusesLoaded: tree.statusesLoaded.value, now: Date.now() }
    const groups: TerminalSessionGroup[] = []
    for (const group of tree.sessionGroups.value) {
      if (group.kind !== 'repo') {
        groups.push(group)
        continue
      }
      const sessions = group.sessions.filter((row) => matchesSessionFilter(row, filter, ctx))
      if (sessions.length) groups.push({ ...group, sessions })
    }
    return groups
  })

  const viewHiddenCount = computed(() => countSessions(tree.sessionGroups.value) - countSessions(viewGroups.value))

  // A repo match carries its whole group, which makes typing a repo name a way
  // to narrow to it. Sessions also match on the slug, the tmux target that a
  // deep link or a script names, and on their tags.
  const groups = computed<TerminalSessionGroup[]>(() => {
    const needle = query.value.trim().toLowerCase()
    if (!needle) return viewGroups.value
    const groups: TerminalSessionGroup[] = []
    for (const group of viewGroups.value) {
      if (group.name.toLowerCase().includes(needle)) {
        groups.push(group)
        continue
      }
      const sessions = group.sessions.filter(
        (row) =>
          row.name.toLowerCase().includes(needle) ||
          row.slug.toLowerCase().includes(needle) ||
          (row.tags ?? []).some((tag) => tag.toLowerCase().includes(needle)),
      )
      if (sessions.length) groups.push({ ...group, sessions })
    }
    return groups
  })

  // A repo defaults open only where something is live, so a machine's worth of
  // dormant repos does not bury the one being worked in. The attached repo
  // counts as live on its own because statuses arrive a poll after the tree.
  const expansion = useTreeExpansion<TerminalSessionGroup>('hive.terminal.sidebar.groups', {
    key: (group) => group.key,
    defaultOpen: (group) => group.kind !== 'repo' || tree.groupAttached(group) || tree.groupRunning(group),
    filtering: () => !!query.value.trim(),
    nodes: () => tree.sessionGroups.value,
  })

  const noMatchesNote = computed(() => {
    const needle = query.value.trim()
    return needle ? `No sessions match “${needle}”.` : 'No sessions match this view.'
  })

  // The placeholder holds until every row's final shape is known, so the tree
  // paints once instead of once per input (list, listings, setting, attach).
  // One-shot: a later reload revalidates the tree already on screen.
  const ready = ref(false)
  watchEffect(() => {
    if (ready.value || !tree.sessionsLoaded.value || !tree.showAllWindowsReady.value) return
    // The view decides which rows exist at all.
    if (options.filterReady && !options.filterReady()) return
    const filter = options.filter?.()
    if (filter && filterReadsStatus(filter) && !tree.statusesLoaded.value) return
    const sweeping =
      tree.attachable.value.length > 0 && (tree.showAllWindows.value || tree.alwaysSwept.value.length > 0)
    if (sweeping && !tree.listingsSettled.value) return
    ready.value = true
  })

  // The pinned sections are exempt from the running filter, so neither can stand
  // in as something it found. A query does reach them, so a query that matched
  // one has found what it went looking for.
  const note = computed<'' | 'empty' | 'no-matches'>(() => {
    if (tree.sessionsError.value || !ready.value) return ''
    if (!tree.activeSessions.value.length) return 'empty'
    const found = query.value.trim() ? groups.value.length > 0 : groups.value.some((group) => group.kind === 'repo')
    return found ? '' : 'no-matches'
  })

  return { query, groups, expansion, viewHiddenCount, noMatchesNote, ready, note }
}
