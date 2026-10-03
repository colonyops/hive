import { useStorage } from '@vueuse/core'
import { computed, ref, watchEffect } from 'vue'
import { useTreeExpansion } from '../../composables/useTreeExpansion'
import type { TerminalSessionGroup } from '../../stores/useTerminalSessions'
import type { TerminalTree } from './useTerminalTree'

function countSessions(groups: TerminalSessionGroup[]): number {
  return groups.reduce((total, group) => total + group.sessions.length, 0)
}

export type SessionTreeView = ReturnType<typeof useSessionTreeView>

/**
 * What the session tree draws: the name query, the running-only narrowing, the
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
) {
  // Transient on purpose: a filter restored at launch hides sessions the user
  // has no reason to suspect are there.
  const query = ref('')
  // Stored, unlike the query, because it is how the user keeps the tree rather
  // than a search to undo. The running note is the price of that: a narrowed
  // tree must never read as a short one.
  const runningOnly = useStorage('hive.terminal.sidebar.running-only', false)

  // Taken before the query, so a repo-name match carries what is left. Inert
  // until the first status poll lands: a session nothing has answered for yet
  // is not a session doing nothing. The pinned sections stay: scratch is the
  // tree's one always-there way to open a shell, and a chat was pinned to be
  // kept in view.
  const runningGroups = computed<TerminalSessionGroup[]>(() => {
    if (!runningOnly.value || !tree.statusesLoaded.value) return tree.sessionGroups.value
    const groups: TerminalSessionGroup[] = []
    for (const group of tree.sessionGroups.value) {
      if (group.kind !== 'repo') {
        groups.push(group)
        continue
      }
      const sessions = group.sessions.filter(tree.rowRunning)
      if (sessions.length) groups.push({ ...group, sessions })
    }
    return groups
  })

  // A repo match carries its whole group, which makes typing a repo name a way
  // to narrow to it. Sessions also match on the slug, the tmux target that a
  // deep link or a script names.
  const groups = computed<TerminalSessionGroup[]>(() => {
    const needle = query.value.trim().toLowerCase()
    if (!needle) return runningGroups.value
    const groups: TerminalSessionGroup[] = []
    for (const group of runningGroups.value) {
      if (group.name.toLowerCase().includes(needle)) {
        groups.push(group)
        continue
      }
      const sessions = group.sessions.filter(
        (row) => row.name.toLowerCase().includes(needle) || row.slug.toLowerCase().includes(needle),
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

  const runningNote = computed(() => {
    if (!runningOnly.value) return ''
    const hidden = countSessions(tree.sessionGroups.value) - countSessions(runningGroups.value)
    return hidden ? `Running sessions only · ${hidden} hidden` : 'Running sessions only'
  })

  const noMatchesNote = computed(() => {
    const needle = query.value.trim()
    return needle ? `No sessions match “${needle}”.` : 'No sessions are running.'
  })

  // The placeholder holds until every row's final shape is known, so the tree
  // paints once instead of once per input (list, listings, setting, attach).
  // One-shot: a later reload revalidates the tree already on screen.
  const ready = ref(false)
  watchEffect(() => {
    if (ready.value || !tree.sessionsLoaded.value || !tree.showAllWindowsReady.value) return
    // The running filter decides which rows exist at all.
    if (runningOnly.value && !tree.statusesLoaded.value) return
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

  return { query, runningOnly, groups, expansion, runningNote, noMatchesNote, ready, note }
}
